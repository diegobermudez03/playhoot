package sessionlifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestManagerAnswerInteraction covers AnswerInteraction's pre-transaction
// validation - resolving interactionUUID to its owning Session - the only
// part mockable without a real DB transaction: the lock, respondent
// authorization, interaction-state handling, and engine execution further
// down this path call the shared sessionlock mechanism package and the real
// Game Language engine directly, which require a real Postgres connection
// to exercise meaningfully. That business logic is proven instead by this
// package's TestManagerAnswerInteraction_Integration* tests against a real
// disposable database.
func TestManagerAnswerInteraction(t *testing.T) {
	repoErr := errors.New("repo failed")

	tests := map[string]func(repo *MockanswerInteractionRepoAPI) require.ErrorAssertionFunc{
		"reports_interaction_not_found": func(repo *MockanswerInteractionRepoAPI) require.ErrorAssertionFunc {
			repo.EXPECT().ResolveSessionForInteraction(gomock.Any(), "missing-uuid").Return(nil, nil)
			return func(tt require.TestingT, err error, _ ...interface{}) {
				require.ErrorIs(tt, err, session.ErrInteractionNotFound)
			}
		},
		"propagates_resolve_error": func(repo *MockanswerInteractionRepoAPI) require.ErrorAssertionFunc {
			repo.EXPECT().ResolveSessionForInteraction(gomock.Any(), "missing-uuid").Return(nil, repoErr)
			return func(tt require.TestingT, err error, _ ...interface{}) {
				require.ErrorIs(tt, err, repoErr)
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockanswerInteractionRepoAPI(ctrl)
			errAssert := setup(repo)

			m := &Manager{answerInteractionRepo: repo}
			_, err := m.AnswerInteraction(context.Background(), "missing-uuid", "user-uuid", nil)
			errAssert(t, err)
		})
	}
}
