package sessionlifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestManagerCreate covers Create's pre-transaction resolution, the only
// part of Create mockable without a real DB transaction: the idempotency
// claim and Host/SessionActor Creation Cycle further down this path call
// the shared sessionlock/idempotency mechanism packages directly, which
// require a real Postgres connection to run their SQL. That business logic
// is proven instead by this package's TestManagerCreate_Integration* tests
// against a real disposable database.
func TestManagerCreate(t *testing.T) {
	type test struct {
		ctx            context.Context
		gameUUID       session.GameUUID
		hostUserUUID   session.UserUUID
		idempotencyKey session.IdempotencyKey
		errAssert      require.ErrorAssertionFunc
	}

	tests := map[string]func(t *testing.T, repo *MockcreateRepoAPI) test{
		"rejects_missing_idempotency_key": func(t *testing.T, repo *MockcreateRepoAPI) test {
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrIdempotencyKeyRequired)
				},
			}
		},
		"returns_game_not_found_when_no_current_version_exists": func(t *testing.T, repo *MockcreateRepoAPI) test {
			repo.EXPECT().ResolveCurrentGameDefinitionUUID(gomock.Any(), "missing-uuid").Return(nil, nil)
			return test{
				ctx: context.Background(), gameUUID: "missing-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrGameNotFound)
				},
			}
		},
		"propagates_resolution_error": func(t *testing.T, repo *MockcreateRepoAPI) test {
			repoErr := errors.New("db unavailable")
			repo.EXPECT().ResolveCurrentGameDefinitionUUID(gomock.Any(), "game-uuid").Return(nil, repoErr)
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, repoErr)
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockcreateRepoAPI(ctrl)
			tc := setup(t, repo)
			m := &Manager{
				createRepo: repo,
			}

			_, err := m.Create(tc.ctx, tc.gameUUID, tc.hostUserUUID, tc.idempotencyKey)
			require.Error(t, err)
			tc.errAssert(t, err)
		})
	}
}
