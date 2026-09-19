package sessionlifecycle

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/management"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestManagerCreate covers Create's pre-transaction validation, the only
// part of Create mockable without a real DB transaction: the idempotency
// claim and Host/SessionActor Creation Cycle further down this path call
// the shared sessionlock/idempotency mechanism packages directly (per
// WORK-0001's Repository Responsibility - Direct mechanism usage), which
// require a real Postgres connection to run their SQL. That business logic
// is proven instead by this package's TestManagerCreate_Integration* tests
// against a real disposable database.
func TestManagerCreate(t *testing.T) {
	type test struct {
		ctx            context.Context
		gameUUID       GameUUID
		hostUserUUID   UserUUID
		idempotencyKey IdempotencyKey
		errAssert      require.ErrorAssertionFunc
	}

	uncompilableDefinition := program.Definition{
		Metadata:     program.Metadata{ID: "broken", Name: "Broken"},
		RootWorkflow: "does-not-exist",
	}

	tests := map[string]func(t *testing.T, gameReader *MockgameCurrentVersionReader) test{
		"rejects_missing_idempotency_key": func(t *testing.T, gameReader *MockgameCurrentVersionReader) test {
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrIdempotencyKeyRequired)
				},
			}
		},
		"returns_game_not_found_when_game_reader_returns_nil": func(t *testing.T, gameReader *MockgameCurrentVersionReader) test {
			gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "missing-uuid").Return(nil, nil)
			return test{
				ctx: context.Background(), gameUUID: "missing-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrGameNotFound)
				},
			}
		},
		"propagates_non_playable_game_error": func(t *testing.T, gameReader *MockgameCurrentVersionReader) test {
			gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(nil, management.ErrNonPlayableGame)
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, management.ErrNonPlayableGame)
				},
			}
		},
		"rejects_definition_that_does_not_compile": func(t *testing.T, gameReader *MockgameCurrentVersionReader) test {
			gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(&management.Game{
				UUID: "game-uuid", VersionUUID: "version-uuid", Definition: uncompilableDefinition,
			}, nil)
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrDefinitionDoesNotCompile)
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			gameReader := NewMockgameCurrentVersionReader(ctrl)
			tc := setup(t, gameReader)
			m := &Manager{
				currentGameReader: gameReader,
			}

			_, err := m.Create(tc.ctx, tc.gameUUID, tc.hostUserUUID, tc.idempotencyKey)
			require.Error(t, err)
			tc.errAssert(t, err)
		})
	}
}
