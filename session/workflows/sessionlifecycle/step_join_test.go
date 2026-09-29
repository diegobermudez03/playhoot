package sessionlifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/session"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestManagerJoin covers Join's pre-transaction validation - idempotency key,
// session.JoinCode resolution, and pinned-artifact loading - the only parts
// of Join mockable without a real DB transaction: the lock, lazy-expiration
// materialization, idempotency claim, and admission decision further down
// this path call the shared sessionlock/idempotency mechanism packages
// directly, which require a real Postgres connection to run their SQL. That
// business logic (replay/conflict, AlreadyJoined, LobbyFull, lazy
// expiration, actor/participant admission) is proven instead by this
// package's TestManagerJoin_Integration* tests against a real disposable
// database.
func TestManagerJoin(t *testing.T) {
	type test struct {
		ctx            context.Context
		joinCode       session.JoinCode
		userUUID       session.UserUUID
		displayName    session.DisplayName
		idempotencyKey session.IdempotencyKey
		errAssert      require.ErrorAssertionFunc
	}

	repoErr := errors.New("repo failed")

	tests := map[string]func(t *testing.T, repo *MockjoinRepoAPI) test{
		"rejects_missing_idempotency_key": func(t *testing.T, repo *MockjoinRepoAPI) test {
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrIdempotencyKeyRequired)
				},
			}
		},
		"rejects_invalid_join_code": func(t *testing.T, repo *MockjoinRepoAPI) test {
			repo.EXPECT().ResolveSessionForJoinCode(gomock.Any(), uint(9999)).Return(nil, nil)
			return test{
				ctx: context.Background(), joinCode: 9999, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrJoinCodeInvalid)
				},
			}
		},
		"alerts_and_rejects_when_pinned_artifact_is_missing": func(t *testing.T, repo *MockjoinRepoAPI) test {
			repo.EXPECT().ResolveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			repo.EXPECT().ResolveGameVersionArtifact(gomock.Any(), "pinned-version-uuid").Return(nil, nil)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrPinnedDefinitionMissing)
				},
			}
		},
		"propagates_resolve_error": func(t *testing.T, repo *MockjoinRepoAPI) test {
			repo.EXPECT().ResolveSessionForJoinCode(gomock.Any(), uint(1234)).Return(nil, repoErr)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, repoErr)
				},
			}
		},
		"propagates_pinned_artifact_read_error": func(t *testing.T, repo *MockjoinRepoAPI) test {
			repo.EXPECT().ResolveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			repo.EXPECT().ResolveGameVersionArtifact(gomock.Any(), "pinned-version-uuid").Return(nil, repoErr)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, repoErr)
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockjoinRepoAPI(ctrl)
			tc := setup(t, repo)
			m := &Manager{joinRepo: repo}

			_, err := m.Join(tc.ctx, tc.joinCode, tc.userUUID, tc.displayName, tc.idempotencyKey)
			require.Error(t, err)
			tc.errAssert(t, err)
		})
	}
}
