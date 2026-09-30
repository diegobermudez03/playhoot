package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestManagerStart_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	t.Run("host_starts_session_and_persists_first_runtime_turn_with_executor_new_state", func(t *testing.T) {
		newState := stateJSON(t, map[string]any{"phase": "running", "score": float64(0)})
		m := New(db, fakeExecutorAlwaysReturning(newState))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
		testfixtures.SeedActiveParticipant(t, db, fx.SessionID, uuid.NewString(), "Player Two")
		testfixtures.SeedJoinCode(t, db, fx.SessionID, 1111, false)

		result, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-1")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeStarted, result.Outcome)
		require.Equal(t, session.SessionUUID(fx.SessionUUID), result.SessionUUID)

		var row struct {
			Phase     string     `gorm:"column:phase"`
			StartedAt *time.Time `gorm:"column:started_at"`
		}
		require.NoError(t, db.Raw(`SELECT phase, started_at FROM sessions WHERE id = ?`, fx.SessionID).Scan(&row).Error)
		require.Equal(t, session.PhaseRunning, row.Phase)
		require.NotNil(t, row.StartedAt)

		var joinCodeRevoked *time.Time
		require.NoError(t, db.Raw(`SELECT revoked_at FROM join_codes WHERE session_id = ?`, fx.SessionID).Scan(&joinCodeRevoked).Error)
		require.NotNil(t, joinCodeRevoked, "the active session.JoinCode must be revoked on Start")

		// Acceptance Criteria: a test proving persisted new_state matches
		// what a live Execute call actually produced - no silent drift
		// between "what was persisted" and "what the Executor returned".
		var turn struct {
			ID       uint   `gorm:"column:id"`
			Sequence uint64 `gorm:"column:sequence"`
			NewState []byte `gorm:"column:new_state"`
		}
		require.NoError(t, db.Raw(`SELECT id, sequence, new_state FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turn).Error)
		require.Equal(t, uint64(1), turn.Sequence)
		require.NotZero(t, turn.ID)
		require.JSONEq(t, string(newState), string(turn.NewState))

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE id = ?`, fx.SessionID).Scan(&currentTurnID).Error)
		require.Equal(t, turn.ID, currentTurnID)

		var activity struct {
			StartedAt         *time.Time `gorm:"column:started_at"`
			ActivityExpiresAt *time.Time `gorm:"column:activity_expires_at"`
		}
		require.NoError(t, db.Raw(`SELECT started_at, activity_expires_at FROM sessions WHERE id = ?`, fx.SessionID).Scan(&activity).Error)
		require.NotNil(t, activity.StartedAt)
		require.NotNil(t, activity.ActivityExpiresAt, "Start must set the initial SESSION-ADR-0013 inactivity deadline")
		require.True(t, activity.ActivityExpiresAt.After(*activity.StartedAt), "activity_expires_at must be started_at plus the configured TTL")
	})

	t.Run("first_turn_requesting_session_complete_terminalizes_session_immediately", func(t *testing.T) {
		newState := stateJSON(t, map[string]any{"done": true})
		m := New(db, fakeExecutorAlwaysReturning(newState, sessionCompleteCommand(t)))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		result, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-terminates")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeStarted, result.Outcome, "the first RuntimeTurn genuinely committed, even though the game ended immediately")
		require.Equal(t, session.TerminalReasonGameCompleted, result.TerminalReason)

		var row struct {
			Phase          string     `gorm:"column:phase"`
			StartedAt      *time.Time `gorm:"column:started_at"`
			TerminalReason *string    `gorm:"column:terminal_reason"`
		}
		require.NoError(t, db.Raw(`SELECT phase, started_at, terminal_reason FROM sessions WHERE id = ?`, fx.SessionID).Scan(&row).Error)
		require.Equal(t, session.PhaseTerminal, row.Phase)
		require.NotNil(t, row.StartedAt, "the Session genuinely started and ran its first Turn before ending")
		require.NotNil(t, row.TerminalReason)
		require.Equal(t, session.TerminalReasonGameCompleted, *row.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "Turn 1 is durably persisted even though the game ended on it")
	})

	t.Run("first_turn_requesting_send_event_returns_it_in_memory_in_order", func(t *testing.T) {
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		hostRef := string(actorRefForActorID(hostActorID))
		payload := stateJSON(t, map[string]any{"sound": "chime"})
		m := New(db, fakeExecutorAlwaysReturning(
			stateJSON(t, map[string]any{}),
			sendEventCommand(t, []string{hostRef}, "welcome", payload),
		))

		result, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-send-event")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeStarted, result.Outcome)
		require.Equal(t, []session.OutboundEvent{
			{Recipients: []session.ActorRef{session.ActorRef(hostRef)}, Name: "welcome", Payload: payload},
		}, result.Events)
	})

	t.Run("retried_start_with_same_idempotency_key_returns_nil_events_on_replay", func(t *testing.T) {
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		hostRef := string(actorRefForActorID(hostActorID))
		m := New(db, fakeExecutorAlwaysReturning(
			stateJSON(t, map[string]any{}),
			sendEventCommand(t, []string{hostRef}, "welcome", stateJSON(t, map[string]any{})),
		))

		first, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-send-event-replay")
		require.NoError(t, err)
		require.NotEmpty(t, first.Events, "the original call must have actually collected the requested event")

		second, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-send-event-replay")
		require.NoError(t, err)
		require.Nil(t, second.Events, "a replayed idempotency claim must never reconstruct/re-emit the original call's transient events")
	})

	t.Run("rejects_start_by_non_host", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, uuid.NewString())
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		notHostUUID := uuid.NewString()
		testfixtures.SeedActiveParticipant(t, db, fx.SessionID, notHostUUID, "Not Host")

		result, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(notHostUUID), "start-key-not-host")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeNotHost, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
		require.Equal(t, session.PhaseLobby, phase, "a rejected Start must not mutate phase")
	})

	t.Run("rejects_start_with_fewer_than_participant_min", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 2, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
		// Only 1 active Participant, below participant_min = 2.

		result, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-not-enough")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeNotEnoughPlayers, result.Outcome)
	})

	t.Run("lazily_materializes_expired_lobby_and_rejects", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(-1*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		result, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-expired")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeLobbyExpired, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
	})

	t.Run("retried_start_with_same_idempotency_key_replays_started", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		first, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-retry")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeStarted, first.Outcome)

		var turnCountAfterFirst int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCountAfterFirst).Error)

		second, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-retry")
		require.NoError(t, err)
		require.Equal(t, first, second, "a same-token retry must replay the exact same recorded outcome")

		var turnCountAfterSecond int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCountAfterSecond).Error)
		require.Equal(t, turnCountAfterFirst, turnCountAfterSecond, "a replay must not re-execute the backend script or create a second Turn")
	})

	t.Run("rejects_conflicting_payload_under_same_token", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		_, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-conflict")
		require.NoError(t, err)

		otherFx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		otherGV := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, otherGV.DefinitionUUID, otherFx.SessionID).Error)
		otherHostActorID := testfixtures.SeedActor(t, db, otherFx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, otherHostActorID, otherFx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, otherHostActorID, "Host")

		_, err = m.Start(context.Background(), session.SessionUUID(otherFx.SessionUUID), session.UserUUID(hostUUID), "start-key-conflict")
		require.ErrorIs(t, err, session.ErrIdempotencyConflict)
	})

	t.Run("fatal_executor_error_terminalizes_and_replays_without_re_executing", func(t *testing.T) {
		m := New(db, fakeExecutorFailing("executor unreachable"))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
		testfixtures.SeedJoinCode(t, db, fx.SessionID, 2222, false)

		first, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-fatal")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeRuntimeInitFailed, first.Outcome)

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

		var failureRow struct {
			FailureKind       string `gorm:"column:failure_kind"`
			ErrorCode         string `gorm:"column:error_code"`
			BaseTurnID        *uint  `gorm:"column:base_turn_id"`
			AttemptedSequence uint64 `gorm:"column:attempted_sequence"`
			SourceKind        string `gorm:"column:source_kind"`
		}
		require.NoError(t, db.Raw(`
			SELECT failure_kind, error_code, base_turn_id, attempted_sequence, source_kind
			FROM session_runtime_failures WHERE session_id = ?
		`, fx.SessionID).Scan(&failureRow).Error)
		require.Equal(t, RuntimeFailureKindExecution, failureRow.FailureKind)
		require.Nil(t, failureRow.BaseTurnID, "no RuntimeTurn ever committed before a pre-first-Turn Start failure")
		require.Equal(t, uint64(1), failureRow.AttemptedSequence)
		require.Equal(t, sessionStartSourceKind, failureRow.SourceKind)
		require.NotEmpty(t, failureRow.ErrorCode)

		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = ?`, fx.SessionID).Scan(&failureCount).Error)
		require.Equal(t, int64(1), failureCount, "a same-token/different-token replay below must not persist a second failure row")

		second, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-fatal")
		require.NoError(t, err)
		require.Equal(t, first, second)

		third, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-key-fatal-different-token")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeRuntimeInitFailed, third.Outcome)
	})

	t.Run("two_concurrent_starts_never_both_execute_a_runtime_turn", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		type outcome struct {
			result session.StartResult
			err    error
		}
		results := make(chan outcome, 2)
		start := func(idempotencyKey session.IdempotencyKey) {
			result, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), idempotencyKey)
			results <- outcome{result: result, err: err}
		}
		go start("start-key-concurrent-a")
		go start("start-key-concurrent-b")

		first := <-results
		second := <-results
		require.NoError(t, first.err)
		require.NoError(t, second.err)
		require.Equal(t, session.StartOutcomeStarted, first.result.Outcome)
		require.Equal(t, session.StartOutcomeStarted, second.result.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "exactly one RuntimeTurn must ever be produced for this Session")
	})
}
