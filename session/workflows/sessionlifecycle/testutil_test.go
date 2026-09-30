package sessionlifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/diegobermudez03/playhoot/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// stateJSON is a small convenience for building a deterministic
// json.RawMessage state value in test cases, standing in for whatever
// opaque shape an authored backend script would actually produce.
func stateJSON(t *testing.T, fields map[string]any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(fields)
	require.NoError(t, err)
	return json.RawMessage(encoded)
}

// fakeExecutorAlwaysReturning builds an executor.Fake whose every Execute
// call succeeds with the given newState/commands, ignoring its input -
// sufficient for tests that only need to prove Session Runtime's own
// persistence/dispatch of an Execute call's result, not the Executor's own,
// separately tested, execution semantics.
func fakeExecutorAlwaysReturning(newState json.RawMessage, commands ...json.RawMessage) *executor.Fake {
	return &executor.Fake{
		ExecuteFunc: func(ctx context.Context, in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: newState, RequestedCommands: commands}, nil
		},
	}
}

// fakeExecutorRejecting builds an executor.Fake whose every Execute call
// returns *executor.ScriptRejectedError - the authored script declining the
// input, an ordinary business outcome distinct from an infrastructure
// failure.
func fakeExecutorRejecting(reason string) *executor.Fake {
	return &executor.Fake{
		ExecuteFunc: func(ctx context.Context, in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{}, &executor.ScriptRejectedError{Reason: reason}
		},
	}
}

// fakeExecutorFailing builds an executor.Fake whose every Execute call
// returns *executor.ExecutorError - an infrastructure-level failure to run
// the script at all.
func fakeExecutorFailing(reason string) *executor.Fake {
	return &executor.Fake{
		ExecuteFunc: func(ctx context.Context, in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{}, &executor.ExecutorError{Reason: reason}
		},
	}
}

// sessionCompleteCommand/sessionFailCommand encode a platform SESSION_
// COMPLETE/SESSION_FAIL Command, for a fakeExecutorAlwaysReturning fixture
// that needs to drive termination.
func sessionCompleteCommand(t *testing.T) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{"kind": "SESSION_COMPLETE"})
	require.NoError(t, err)
	return encoded
}

func sessionFailCommand(t *testing.T, reason string) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{"kind": "SESSION_FAIL", "reason": reason})
	require.NoError(t, err)
	return encoded
}

// scheduleTimerCommand/cancelTimerCommand encode a platform SCHEDULE_TIMER/
// CANCEL_TIMER Command, for a fakeExecutorAlwaysReturning fixture that needs
// to drive timer-obligation persistence.
func scheduleTimerCommand(t *testing.T, timer string, delayMs int64) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"kind": "SCHEDULE_TIMER", "timer": timer, "delay_ms": delayMs})
	require.NoError(t, err)
	return encoded
}

func cancelTimerCommand(t *testing.T, timer string) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{"kind": "CANCEL_TIMER", "timer": timer})
	require.NoError(t, err)
	return encoded
}

// seedRunningSessionWithHost seeds a RUNNING Session (bypassing Start
// entirely, by directly seeding session_runtime_turns/sessions rows) owned
// by a fresh host actor, with activityExpiresAt far enough in the future to
// never lazily materialize as inactive during a test. Returns the Session's
// identity, the host's UserUUID, and the seeded current Turn's id/sequence
// for a test that needs to assert against them directly.
func seedRunningSessionWithHost(t *testing.T, db *gorm.DB, currentState json.RawMessage) (fx testfixtures.SessionFixture, hostUUID session.UserUUID, hostActorID uint, turnID uint) {
	t.Helper()

	fx = testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	hostUUIDStr := uuid.NewString()
	hostActorID = testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
	require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
	testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

	turnID = testfixtures.SeedRuntimeTurn(t, db, fx.SessionID, 1, sessionStartSourceKind, currentState)
	activityExpiresAt := time.Now().Add(1 * time.Hour)
	require.NoError(t, db.Exec(`
		UPDATE sessions
		SET phase = ?, started_at = CURRENT_TIMESTAMP, activity_expires_at = ?, current_turn_id = ?
		WHERE id = ?
	`, session.PhaseRunning, activityExpiresAt, turnID, fx.SessionID).Error)

	return fx, session.UserUUID(hostUUIDStr), hostActorID, turnID
}

// sessionIDForUUID resolves sessionUUID's internal id, for a test assertion
// that needs to query a satellite table by session_id.
func sessionIDForUUID(t *testing.T, db *gorm.DB, sessionUUID session.SessionUUID) uint {
	t.Helper()
	var id uint
	require.NoError(t, db.Raw(`SELECT id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&id).Error)
	return id
}

// activityExpiresAtForUUID reads sessionUUID's current
// sessions.activity_expires_at, for a test asserting a Turn-committing
// RUNNING-phase operation renewed it.
func activityExpiresAtForUUID(t *testing.T, db *gorm.DB, sessionUUID session.SessionUUID) *time.Time {
	t.Helper()
	var deadline *time.Time
	require.NoError(t, db.Raw(`SELECT activity_expires_at FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&deadline).Error)
	return deadline
}

// startedSessionWithExecutor seeds a LOBBY Session with one active host
// Participant and a pinned session_game_version_artifacts row, starts it
// (against exec's own Start-time behavior), and returns the resulting
// Manager plus the Session's/host's identities - the common setup every
// RUNNING-phase integration test below builds on. A caller that needs
// different Executor behavior for its own post-Start call builds its own
// executor.Fake whose ExecuteFunc branches on the incoming Event's own
// "kind", rather than call ordering.
func startedSessionWithExecutor(t *testing.T, db *gorm.DB, exec executor.Executor) (m *Manager, sessionUUID session.SessionUUID, hostUUID session.UserUUID) {
	t.Helper()

	m = New(db, exec)

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
	require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
	hostUUIDStr := uuid.NewString()
	hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
	require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
	testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

	startResult, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr), session.IdempotencyKey(uuid.NewString()))
	require.NoError(t, err)
	require.Equal(t, session.StartOutcomeStarted, startResult.Outcome)

	return m, session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr)
}

// seedLobbySessionWithParticipantMax seeds a LOBBY Session pinned to a
// fresh session_game_version_artifacts row carrying participantMax (nil
// means unlimited), for a Join/Start test that needs to control the
// pinned capacity independently of "the Game's current version".
func seedLobbySessionWithParticipantMax(t *testing.T, db *gorm.DB, lobbyExpiresAt time.Time, participantMax *int) testfixtures.SessionFixture {
	t.Helper()

	fx := testfixtures.SeedLobbySession(t, db, lobbyExpiresAt)
	gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, participantMax)
	require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
	fx.GameDefinitionUUID = gv.DefinitionUUID
	return fx
}

// intPtr is a small literal-int-pointer convenience for
// ParticipantConstraints.Max-shaped test fixtures.
func intPtr(v int) *int { return &v }

// eventKind decodes raw's own "kind" discriminator, for an executor.Fake
// whose ExecuteFunc needs to behave differently for SESSION_STARTED versus
// a later Event against the same fixed script/artifact.
func eventKind(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var envelope struct {
		Kind string `json:"kind"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	return envelope.Kind
}
