package sessionlifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestManagerJoin_Integration_ConcurrentJoinsForFinalSlot proves - against a
// real Postgres database - that when only one lobby slot remains, exactly
// one of two concurrent Joins succeeds, via the per-Session DB lock rather
// than an application-level check-then-act race.
func TestManagerJoin_Integration_ConcurrentJoinsForFinalSlot(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 6001, false)
	m := New(db, nil, stubPinnedGameReader{playersMax: 1})

	userA := UserUUID(uuid.NewString())
	userB := UserUUID(uuid.NewString())

	start := make(chan struct{})
	var wg sync.WaitGroup
	var resA, resB JoinResult
	var errA, errB error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		resA, errA = m.Join(context.Background(), 6001, userA, "A", "concurrent-join-a")
	}()
	go func() {
		defer wg.Done()
		<-start
		resB, errB = m.Join(context.Background(), 6001, userB, "B", "concurrent-join-b")
	}()
	close(start)
	wg.Wait()

	successCount := 0
	if errA == nil {
		successCount++
		require.NotEmpty(t, resA.DisplayName)
	} else {
		require.ErrorIs(t, errA, session.ErrLobbyFull)
	}
	if errB == nil {
		successCount++
		require.NotEmpty(t, resB.DisplayName)
	} else {
		require.ErrorIs(t, errB, session.ErrLobbyFull)
	}
	require.Equal(t, 1, successCount, "exactly one Join must succeed when only one slot remains")

	var activeCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM session_participants p
		INNER JOIN session_actors a ON a.id = p.session_actor_id
		WHERE a.session_id = ? AND p.active = TRUE
	`, fx.SessionID).Scan(&activeCount).Error)
	require.Equal(t, int64(1), activeCount)
}

// TestManagerJoin_Integration_ConcurrentJoinRacingLeave proves - through the
// real public Join/Leave Manager methods against a real Postgres database -
// that a Join racing a Leave around capacity resolves correctly under the
// same per-Session DB lock: whichever operation wins serialization
// determines a consistent, non-corrupted final state.
func TestManagerJoin_Integration_ConcurrentJoinRacingLeave(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := New(db, nil, stubPinnedGameReader{playersMax: 1})

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 7001, false)

	existingUser := UserUUID(uuid.NewString())
	testfixtures.SeedActiveParticipant(t, db, fx.SessionID, string(existingUser), "Existing")
	newUser := UserUUID(uuid.NewString())

	start := make(chan struct{})
	var wg sync.WaitGroup
	var joinResult JoinResult
	var joinErr, leaveErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		joinResult, joinErr = m.Join(context.Background(), 7001, newUser, "New", "race-join")
	}()
	go func() {
		defer wg.Done()
		<-start
		_, leaveErr = m.Leave(context.Background(), SessionUUID(fx.SessionUUID), existingUser, "race-leave")
	}()
	close(start)
	wg.Wait()

	require.NoError(t, leaveErr, "Leave has no reason to fail regardless of ordering")

	var activeCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM session_participants p
		INNER JOIN session_actors a ON a.id = p.session_actor_id
		WHERE a.session_id = ? AND p.active = TRUE
	`, fx.SessionID).Scan(&activeCount).Error)

	if joinErr == nil {
		require.NotEmpty(t, joinResult.DisplayName)
		require.Equal(t, int64(1), activeCount, "Join won the race after Leave released the slot")
	} else {
		require.ErrorIs(t, joinErr, session.ErrLobbyFull)
		require.Equal(t, int64(0), activeCount, "Join lost the race while the existing Participant still held the only slot")
	}
}

// TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration proves
// that two concurrent operations racing an already-expired (but not yet
// materialized) lobby never revive it: both must observe the expiration, and
// the Session ends up TERMINAL exactly once, via the same per-Session lock
// rather than two independent lazy-materialization attempts corrupting each
// other.
func TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := New(db, nil, stubPinnedGameReader{playersMax: 4})

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(-1*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 7002, false)

	existingUser := UserUUID(uuid.NewString())
	testfixtures.SeedActiveParticipant(t, db, fx.SessionID, string(existingUser), "Existing")
	newUser := UserUUID(uuid.NewString())

	start := make(chan struct{})
	var wg sync.WaitGroup
	var joinErr, leaveErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, joinErr = m.Join(context.Background(), 7002, newUser, "New", "race-expire-join")
	}()
	go func() {
		defer wg.Done()
		<-start
		_, leaveErr = m.Leave(context.Background(), SessionUUID(fx.SessionUUID), existingUser, "race-expire-leave")
	}()
	close(start)
	wg.Wait()

	require.ErrorIs(t, joinErr, session.ErrLobbyExpired)
	require.ErrorIs(t, leaveErr, session.ErrNotInLobbyPhase)

	var phase, terminalReason string
	require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
	require.NoError(t, db.Raw(`SELECT terminal_reason FROM sessions WHERE id = ?`, fx.SessionID).Scan(&terminalReason).Error)
	require.Equal(t, session.PhaseTerminal, phase)
	require.Equal(t, session.TerminalReasonLobbyExpired, terminalReason)
}
