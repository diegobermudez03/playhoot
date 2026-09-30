package sessionlifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/executor"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

// fakeDBServicer is GetClientState's own minimal dbServicer stub: unlike
// every mutating step, GetClientState never opens a transaction, so the
// *gorm.DB it forwards to its (fully mocked) repo calls is never actually
// dereferenced.
type fakeDBServicer struct{}

func (fakeDBServicer) GetDB() *gorm.DB { return nil }

// TestManagerGetClientState covers GetClientState end to end against
// mocked persistence and a fake Executor - unlike every mutating step, it
// opens no DB transaction at all (a pure read), so its full business logic
// is mockable here rather than requiring a real Postgres integration test.
func TestManagerGetClientState(t *testing.T) {
	repoErr := errors.New("repo failed")
	turnID := uint(7)

	tests := map[string]func(t *testing.T, repo *MockgetClientStateRepoAPI) (executor.Executor, func(session.GetClientStateResult, error)){
		"session_not_found": func(t *testing.T, repo *MockgetClientStateRepoAPI) (executor.Executor, func(session.GetClientStateResult, error)) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "missing-uuid").Return(nil, nil)
			return nil, func(_ session.GetClientStateResult, err error) {
				require.ErrorIs(t, err, session.ErrSessionNotFound)
			}
		},
		"propagates_resolve_error": func(t *testing.T, repo *MockgetClientStateRepoAPI) (executor.Executor, func(session.GetClientStateResult, error)) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "missing-uuid").Return(nil, repoErr)
			return nil, func(_ session.GetClientStateResult, err error) {
				require.ErrorIs(t, err, repoErr)
			}
		},
		"not_running_when_no_current_turn": func(t *testing.T, repo *MockgetClientStateRepoAPI) (executor.Executor, func(session.GetClientStateResult, error)) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "missing-uuid").Return(&internalrepo.Session{ID: 1}, nil)
			return nil, func(result session.GetClientStateResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetClientStateOutcomeNotRunning, result.Outcome)
			}
		},
		"not_a_participant_when_actor_missing": func(t *testing.T, repo *MockgetClientStateRepoAPI) (executor.Executor, func(session.GetClientStateResult, error)) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "missing-uuid").Return(&internalrepo.Session{ID: 1, CurrentTurnID: &turnID}, nil)
			repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(1), "user-uuid").Return(nil, nil)
			return nil, func(result session.GetClientStateResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetClientStateOutcomeNotAParticipant, result.Outcome)
			}
		},
		"reports_pinned_artifact_missing": func(t *testing.T, repo *MockgetClientStateRepoAPI) (executor.Executor, func(session.GetClientStateResult, error)) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "missing-uuid").Return(&internalrepo.Session{ID: 1, GameDefinitionUUID: "def-uuid", CurrentTurnID: &turnID}, nil)
			repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(1), "user-uuid").Return(&internalrepo.Actor{ID: 42}, nil)
			repo.EXPECT().GetRuntimeTurn(gomock.Any(), gomock.Any(), turnID).Return(&internalrepo.RuntimeTurn{ID: turnID, NewState: json.RawMessage(`{}`)}, nil)
			repo.EXPECT().ResolveGameVersionArtifact(gomock.Any(), "def-uuid").Return(nil, nil)
			return nil, func(_ session.GetClientStateResult, err error) {
				require.ErrorIs(t, err, session.ErrPinnedDefinitionMissing)
			}
		},
		"projection_rejected_by_script": func(t *testing.T, repo *MockgetClientStateRepoAPI) (executor.Executor, func(session.GetClientStateResult, error)) {
			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "missing-uuid").Return(&internalrepo.Session{ID: 1, GameDefinitionUUID: "def-uuid", CurrentTurnID: &turnID}, nil)
			repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(1), "user-uuid").Return(&internalrepo.Actor{ID: 42}, nil)
			repo.EXPECT().GetRuntimeTurn(gomock.Any(), gomock.Any(), turnID).Return(&internalrepo.RuntimeTurn{ID: turnID, NewState: json.RawMessage(`{}`)}, nil)
			repo.EXPECT().ResolveGameVersionArtifact(gomock.Any(), "def-uuid").Return(&internalrepo.GameVersionArtifact{BackendScript: "function project() {}"}, nil)
			exec := &executor.Fake{ProjectFunc: func(ctx context.Context, in executor.ProjectionInput) (executor.ProjectionOutput, error) {
				return executor.ProjectionOutput{}, &executor.ScriptRejectedError{Reason: "no project function"}
			}}
			return exec, func(result session.GetClientStateResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetClientStateOutcomeProjectionRejected, result.Outcome)
			}
		},
		"computed_from_filtered_projection_input": func(t *testing.T, repo *MockgetClientStateRepoAPI) (executor.Executor, func(session.GetClientStateResult, error)) {
			state := json.RawMessage(`{"players":{"42":{"hand":["A"]},"99":{"hand":["7","2"]}}}`)
			visibility := json.RawMessage(`{"rules":[{"path":"players.*.hand","class":"private"}]}`)

			repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "missing-uuid").Return(&internalrepo.Session{ID: 1, GameDefinitionUUID: "def-uuid", CurrentTurnID: &turnID}, nil)
			repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(1), "user-uuid").Return(&internalrepo.Actor{ID: 42}, nil)
			repo.EXPECT().GetRuntimeTurn(gomock.Any(), gomock.Any(), turnID).Return(&internalrepo.RuntimeTurn{ID: turnID, Sequence: 3, NewState: state}, nil)
			repo.EXPECT().ResolveGameVersionArtifact(gomock.Any(), "def-uuid").Return(&internalrepo.GameVersionArtifact{
				BackendScript:        "function project(state, viewer, context) { return state; }",
				ProjectionVisibility: visibility,
			}, nil)

			var capturedState json.RawMessage
			exec := &executor.Fake{ProjectFunc: func(ctx context.Context, in executor.ProjectionInput) (executor.ProjectionOutput, error) {
				capturedState = in.State
				require.Equal(t, "42", in.Viewer)
				return executor.ProjectionOutput{ClientState: in.State}, nil
			}}
			return exec, func(result session.GetClientStateResult, err error) {
				require.NoError(t, err)
				require.Equal(t, session.GetClientStateOutcomeComputed, result.Outcome)
				require.Equal(t, uint64(3), result.Revision, "Revision must be the loaded RuntimeTurn's own Sequence")

				var parsed map[string]interface{}
				require.NoError(t, json.Unmarshal(capturedState, &parsed))
				players := parsed["players"].(map[string]interface{})
				// Viewer 42 sees its own hand...
				require.NotNil(t, players["42"])
				// ...but never viewer 99's private hand - the Executor
				// itself never received it, since Filter excluded it
				// before this call was ever made.
				require.Nil(t, players["99"])
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockgetClientStateRepoAPI(ctrl)
			exec, assert := setup(t, repo)

			m := &Manager{getClientStateRepo: repo, dbServicer: fakeDBServicer{}, executor: exec}
			result, err := m.GetClientState(context.Background(), "missing-uuid", "user-uuid")
			assert(result, err)
		})
	}
}
