package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestManagerStart_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	t.Run("host_starts_session_and_persists_first_runtime_turn", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: startableDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
		testfixtures.SeedActiveParticipant(t, db, fx.SessionID, uuid.NewString(), "Player Two")
		testfixtures.SeedJoinCode(t, db, fx.SessionID, 1111, false)

		result, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), "start-key-1")
		require.NoError(t, err)
		require.Equal(t, StartOutcomeStarted, result.Outcome)
		require.Equal(t, SessionUUID(fx.SessionUUID), result.SessionUUID)

		var row struct {
			Phase     string     `gorm:"column:phase"`
			StartedAt *time.Time `gorm:"column:started_at"`
		}
		require.NoError(t, db.Raw(`SELECT phase, started_at FROM sessions WHERE id = ?`, fx.SessionID).Scan(&row).Error)
		require.Equal(t, session.PhaseRunning, row.Phase)
		require.NotNil(t, row.StartedAt)

		var joinCodeRevoked *time.Time
		require.NoError(t, db.Raw(`SELECT revoked_at FROM join_codes WHERE session_id = ?`, fx.SessionID).Scan(&joinCodeRevoked).Error)
		require.NotNil(t, joinCodeRevoked, "the active JoinCode must be revoked on Start")

		var turn struct {
			ID       uint   `gorm:"column:id"`
			Sequence uint64 `gorm:"column:sequence"`
		}
		require.NoError(t, db.Raw(`SELECT id, sequence FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turn).Error)
		require.Equal(t, uint64(1), turn.Sequence)
		require.NotZero(t, turn.ID)

		var stepCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_steps WHERE runtime_turn_id = ?`, turn.ID).Scan(&stepCount).Error)
		require.Equal(t, int64(1), stepCount)

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE id = ?`, fx.SessionID).Scan(&currentTurnID).Error)
		require.Equal(t, turn.ID, currentTurnID)
	})

	t.Run("rejects_start_by_non_host", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: startableDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, uuid.NewString())
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		notHostUUID := uuid.NewString()
		testfixtures.SeedActiveParticipant(t, db, fx.SessionID, notHostUUID, "Not Host")

		result, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(notHostUUID), "start-key-not-host")
		require.NoError(t, err)
		require.Equal(t, StartOutcomeNotHost, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
		require.Equal(t, session.PhaseLobby, phase, "a rejected Start must not mutate phase")
	})

	t.Run("rejects_start_with_fewer_than_players_min", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: startableDefinition(2, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
		// Only 1 active Participant, below players.min = 2.

		result, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), "start-key-not-enough")
		require.NoError(t, err)
		require.Equal(t, StartOutcomeNotEnoughPlayers, result.Outcome)
	})

	t.Run("lazily_materializes_expired_lobby_and_rejects", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: startableDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(-1*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		result, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), "start-key-expired")
		require.NoError(t, err)
		require.Equal(t, StartOutcomeLobbyExpired, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
	})

	t.Run("retried_start_with_same_idempotency_key_replays_started", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: startableDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		first, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), "start-key-retry")
		require.NoError(t, err)
		require.Equal(t, StartOutcomeStarted, first.Outcome)

		var turnCountAfterFirst int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCountAfterFirst).Error)

		second, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), "start-key-retry")
		require.NoError(t, err)
		require.Equal(t, first, second, "a same-token retry must replay the exact same recorded outcome")

		var turnCountAfterSecond int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCountAfterSecond).Error)
		require.Equal(t, turnCountAfterFirst, turnCountAfterSecond, "a replay must not re-execute engine initialization or create a second Turn")
	})

	t.Run("rejects_conflicting_payload_under_same_token", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: startableDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		_, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), "start-key-conflict")
		require.NoError(t, err)

		otherFx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		otherHostActorID := testfixtures.SeedActor(t, db, otherFx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, otherHostActorID, otherFx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, otherHostActorID, "Host")

		_, err = m.Start(context.Background(), SessionUUID(otherFx.SessionUUID), UserUUID(hostUUID), "start-key-conflict")
		require.ErrorIs(t, err, session.ErrIdempotencyConflict)
	})

	t.Run("fatal_runtime_init_failure_terminalizes_and_replays_without_re_executing", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: nonStartableDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
		testfixtures.SeedJoinCode(t, db, fx.SessionID, 2222, false)

		first, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), "start-key-fatal")
		require.NoError(t, err)
		require.Equal(t, StartOutcomeRuntimeInitFailed, first.Outcome)

		var row struct {
			Phase          string     `gorm:"column:phase"`
			StartedAt      *time.Time `gorm:"column:started_at"`
			TerminalReason *string    `gorm:"column:terminal_reason"`
		}
		require.NoError(t, db.Raw(`SELECT phase, started_at, terminal_reason FROM sessions WHERE id = ?`, fx.SessionID).Scan(&row).Error)
		require.Equal(t, session.PhaseTerminal, row.Phase)
		require.Nil(t, row.StartedAt, "a fatally-failed Start must never set started_at")
		require.NotNil(t, row.TerminalReason)
		require.Equal(t, session.TerminalReasonRuntimeExecutionFailed, *row.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCount).Error)
		require.Zero(t, turnCount, "no RuntimeTurn is ever written for a pre-first-Turn fatal failure")

		var joinCodeRevoked *time.Time
		require.NoError(t, db.Raw(`SELECT revoked_at FROM join_codes WHERE session_id = ?`, fx.SessionID).Scan(&joinCodeRevoked).Error)
		require.NotNil(t, joinCodeRevoked)

		// A same-token retry against the now-TERMINAL Session must replay
		// the recorded outcome, never re-attempt engine initialization.
		second, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), "start-key-fatal")
		require.NoError(t, err)
		require.Equal(t, first, second)

		// A *differently*-tokened Start against the same already-fatally-
		// terminalized Session (no prior claim for this token, so no replay
		// applies) must still report RuntimeInitFailed, never LobbyExpired -
		// the Session is TERMINAL because Start's own initialization
		// deterministically failed, not because the lobby timed out.
		third, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), "start-key-fatal-different-token")
		require.NoError(t, err)
		require.Equal(t, StartOutcomeRuntimeInitFailed, third.Outcome)
	})

	t.Run("two_concurrent_starts_never_both_execute_a_runtime_turn", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: startableDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		type outcome struct {
			result StartResult
			err    error
		}
		results := make(chan outcome, 2)
		start := func(idempotencyKey IdempotencyKey) {
			result, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), idempotencyKey)
			results <- outcome{result: result, err: err}
		}
		go start("start-key-concurrent-a")
		go start("start-key-concurrent-b")

		first := <-results
		second := <-results
		require.NoError(t, first.err)
		require.NoError(t, second.err)
		require.Equal(t, StartOutcomeStarted, first.result.Outcome)
		require.Equal(t, StartOutcomeStarted, second.result.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "exactly one RuntimeTurn must ever be produced for this Session")
	})
}
