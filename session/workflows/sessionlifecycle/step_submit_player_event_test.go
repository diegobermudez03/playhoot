package sessionlifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/session"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestManagerSubmitPlayerEvent covers SubmitPlayerEvent's pre-transaction
// validation - the only part mockable without a real DB transaction: the
// lock, idempotency claim, actor resolution, and Executor call further down
// this path call the shared sessionlock/idempotency mechanism packages and
// the injected executor.Executor directly, which require a real Postgres
// connection to exercise meaningfully. That business logic is proven
// instead by this package's TestManagerSubmitPlayerEvent_Integration* tests
// against a real disposable database.
func TestManagerSubmitPlayerEvent(t *testing.T) {
	m := &Manager{}

	_, err := m.SubmitPlayerEvent(context.Background(), "session-uuid", "user-uuid", "Guess", nil, "")
	require.ErrorIs(t, err, session.ErrIdempotencyKeyRequired)
}

// TestManagerGetSubmitPlayerEventOutcome covers GetSubmitPlayerEventOutcome
// end to end against mocked persistence - unlike SubmitPlayerEvent itself,
// it opens no DB transaction at all (a pure read), so its full business
// logic is mockable here rather than requiring a real Postgres integration
// test, mirroring TestManagerGetClientState's own style.
func TestManagerGetSubmitPlayerEventOutcome(t *testing.T) {
	repoErr := errors.New("repo failed")
	sessionID := uint(1)

	tests := map[string]func(t *testing.T, repo *MocksubmitPlayerEventRepoAPI) func(session.GetSubmitPlayerEventOutcomeResult, error){
		"session_not_found": func(t *testing.T, repo *MocksubmitPlayerEventRepoAPI) func(session.GetSubmitPlayerEventOutcomeResult, error) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "session-uuid").Return(nil, nil)
			return func(_ session.GetSubmitPlayerEventOutcomeResult, err error) {
				require.ErrorIs(t, err, session.ErrSessionNotFound)
			}
		},
		"propagates_resolve_error": func(t *testing.T, repo *MocksubmitPlayerEventRepoAPI) func(session.GetSubmitPlayerEventOutcomeResult, error) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "session-uuid").Return(nil, repoErr)
			return func(_ session.GetSubmitPlayerEventOutcomeResult, err error) {
				require.ErrorIs(t, err, repoErr)
			}
		},
		"not_found_when_no_request_row": func(t *testing.T, repo *MocksubmitPlayerEventRepoAPI) func(session.GetSubmitPlayerEventOutcomeResult, error) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "session-uuid").Return(&internalrepo.Session{ID: sessionID}, nil)
			repo.EXPECT().FindSessionRequest(gomock.Any(), gomock.Any(), "user-uuid", operationSubmitPlayerEvent, "idem-key").Return(nil, nil)
			return func(result session.GetSubmitPlayerEventOutcomeResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetSubmitPlayerEventOutcomeNotFound, result.Outcome)
			}
		},
		"not_found_when_still_pending": func(t *testing.T, repo *MocksubmitPlayerEventRepoAPI) func(session.GetSubmitPlayerEventOutcomeResult, error) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "session-uuid").Return(&internalrepo.Session{ID: sessionID}, nil)
			repo.EXPECT().FindSessionRequest(gomock.Any(), gomock.Any(), "user-uuid", operationSubmitPlayerEvent, "idem-key").Return(&internalrepo.Request{Status: internalrepo.RequestStatusPending, SessionID: &sessionID}, nil)
			return func(result session.GetSubmitPlayerEventOutcomeResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetSubmitPlayerEventOutcomeNotFound, result.Outcome)
			}
		},
		"not_found_when_request_belongs_to_a_different_session": func(t *testing.T, repo *MocksubmitPlayerEventRepoAPI) func(session.GetSubmitPlayerEventOutcomeResult, error) {
			otherSessionID := uint(2)
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "session-uuid").Return(&internalrepo.Session{ID: sessionID}, nil)
			repo.EXPECT().FindSessionRequest(gomock.Any(), gomock.Any(), "user-uuid", operationSubmitPlayerEvent, "idem-key").Return(&internalrepo.Request{Status: internalrepo.RequestStatusCompleted, SessionID: &otherSessionID, Outcome: submitPlayerEventOutcomeRejected}, nil)
			return func(result session.GetSubmitPlayerEventOutcomeResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetSubmitPlayerEventOutcomeNotFound, result.Outcome)
			}
		},
		"found_rejected": func(t *testing.T, repo *MocksubmitPlayerEventRepoAPI) func(session.GetSubmitPlayerEventOutcomeResult, error) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "session-uuid").Return(&internalrepo.Session{ID: sessionID}, nil)
			repo.EXPECT().FindSessionRequest(gomock.Any(), gomock.Any(), "user-uuid", operationSubmitPlayerEvent, "idem-key").Return(&internalrepo.Request{Status: internalrepo.RequestStatusCompleted, SessionID: &sessionID, Outcome: submitPlayerEventOutcomeRejected}, nil)
			return func(result session.GetSubmitPlayerEventOutcomeResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetSubmitPlayerEventOutcomeFound, result.Outcome)
				require.Equal(t, session.SubmitPlayerEventResult{Outcome: session.SubmitPlayerEventOutcomeRejected}, result.Result)
			}
		},
		"found_runtime_execution_failed": func(t *testing.T, repo *MocksubmitPlayerEventRepoAPI) func(session.GetSubmitPlayerEventOutcomeResult, error) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "session-uuid").Return(&internalrepo.Session{ID: sessionID}, nil)
			repo.EXPECT().FindSessionRequest(gomock.Any(), gomock.Any(), "user-uuid", operationSubmitPlayerEvent, "idem-key").Return(&internalrepo.Request{Status: internalrepo.RequestStatusCompleted, SessionID: &sessionID, Outcome: submitPlayerEventOutcomeRuntimeExecutionFailed}, nil)
			return func(result session.GetSubmitPlayerEventOutcomeResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetSubmitPlayerEventOutcomeFound, result.Outcome)
				require.Equal(t, session.SubmitPlayerEventResult{Outcome: session.SubmitPlayerEventOutcomeRuntimeExecutionFailed}, result.Result)
			}
		},
		"found_accepted_decodes_original_response_payload": func(t *testing.T, repo *MocksubmitPlayerEventRepoAPI) func(session.GetSubmitPlayerEventOutcomeResult, error) {
			original := session.SubmitPlayerEventResult{Outcome: session.SubmitPlayerEventOutcomeAccepted, SessionUUID: "session-uuid", TerminalReason: "GAME_COMPLETED"}
			encoded, err := json.Marshal(original)
			require.NoError(t, err)
			payload := string(encoded)
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "session-uuid").Return(&internalrepo.Session{ID: sessionID}, nil)
			repo.EXPECT().FindSessionRequest(gomock.Any(), gomock.Any(), "user-uuid", operationSubmitPlayerEvent, "idem-key").Return(&internalrepo.Request{Status: internalrepo.RequestStatusCompleted, SessionID: &sessionID, Outcome: submitPlayerEventOutcomeAccepted, ResponsePayload: &payload}, nil)
			return func(result session.GetSubmitPlayerEventOutcomeResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetSubmitPlayerEventOutcomeFound, result.Outcome)
				require.Equal(t, original, result.Result, "must recover the exact same outcome the original SubmitPlayerEvent call itself returned")
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocksubmitPlayerEventRepoAPI(ctrl)
			assert := setup(t, repo)

			m := &Manager{submitPlayerEventRepo: repo, dbServicer: fakeDBServicer{}}
			result, err := m.GetSubmitPlayerEventOutcome(context.Background(), "session-uuid", "user-uuid", "idem-key")
			assert(result, err)
		})
	}
}
