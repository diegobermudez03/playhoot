package joinsession_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/diegobermudez03/playhoot/game/session/usecases/joinsession"
	"github.com/diegobermudez03/playhoot/game/session/usecases/leavesession"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// stubGameDefinitionResolver satisfies joinsession's gameDefinitionResolver
// collaborator with a fixed pinned Definition, so these tests can exercise
// the real public JoinSession/LeaveSession UseCases end to end against a
// real database without depending on Game Management.
type stubGameDefinitionResolver struct {
	playersMax int
}

func (s stubGameDefinitionResolver) GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error) {
	return &program.Definition{Players: program.PlayerPolicy{Max: s.playersMax}}, nil
}

// TestJoinSession_ConcurrentJoinRacingLeave proves - through the real public
// JoinSession/LeaveSession UseCases against a real Postgres database - that a
// Join racing a Leave around capacity resolves correctly under the same
// per-Session DB lock: whichever operation wins serialization determines a
// consistent, non-corrupted final state.
func TestJoinSession_ConcurrentJoinRacingLeave(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	joinUseCase := joinsession.New(db, stubGameDefinitionResolver{playersMax: 1})
	leaveUseCase := leavesession.New(db)

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 7001, false)

	existingUser := uuid.NewString()
	testfixtures.SeedActiveParticipant(t, db, fx.SessionID, existingUser, "Existing")
	newUser := uuid.NewString()

	start := make(chan struct{})
	var wg sync.WaitGroup
	var joinResult joinsession.Result
	var joinErr, leaveErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		joinResult, joinErr = joinUseCase.JoinSession(context.Background(), joinsession.Input{
			JoinCode: 7001, UserUUID: newUser, DisplayName: "New", IdempotencyKey: "race-join",
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		_, leaveErr = leaveUseCase.LeaveSession(context.Background(), leavesession.Input{
			SessionUUID: fx.SessionUUID, UserUUID: existingUser, IdempotencyKey: "race-leave",
		})
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

// TestJoinSession_ConcurrentOperationRacingLobbyExpiration proves that two
// concurrent operations racing an already-expired (but not yet materialized)
// lobby never revive it: both must observe the expiration, and the Session
// ends up TERMINAL exactly once, via the same per-Session lock rather than
// two independent lazy-materialization attempts corrupting each other.
func TestJoinSession_ConcurrentOperationRacingLobbyExpiration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	joinUseCase := joinsession.New(db, stubGameDefinitionResolver{playersMax: 4})
	leaveUseCase := leavesession.New(db)

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(-1*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 7002, false)

	existingUser := uuid.NewString()
	testfixtures.SeedActiveParticipant(t, db, fx.SessionID, existingUser, "Existing")
	newUser := uuid.NewString()

	start := make(chan struct{})
	var wg sync.WaitGroup
	var joinErr, leaveErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, joinErr = joinUseCase.JoinSession(context.Background(), joinsession.Input{
			JoinCode: 7002, UserUUID: newUser, DisplayName: "New", IdempotencyKey: "race-expire-join",
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		_, leaveErr = leaveUseCase.LeaveSession(context.Background(), leavesession.Input{
			SessionUUID: fx.SessionUUID, UserUUID: existingUser, IdempotencyKey: "race-expire-leave",
		})
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
