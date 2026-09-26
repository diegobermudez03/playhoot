package sessionlifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestManagerExpireTimer covers ExpireTimer's pre-transaction validation -
// resolving timerObligationUUID to its owning Session - the only part
// mockable without a real DB transaction, mirroring
// TestManagerAnswerInteraction exactly: the lock, obligation-state handling,
// and engine execution further down this path call the shared sessionlock
// mechanism package and the real Game Language engine directly, which
// require a real Postgres connection to exercise meaningfully. That business
// logic is proven instead by this package's
// TestManagerExpireTimer_Integration* tests against a real disposable
// database.
func TestManagerExpireTimer(t *testing.T) {
	repoErr := errors.New("repo failed")

	tests := map[string]func(repo *MockexpireTimerRepoAPI) require.ErrorAssertionFunc{
		"reports_timer_obligation_not_found": func(repo *MockexpireTimerRepoAPI) require.ErrorAssertionFunc {
			repo.EXPECT().ResolveSessionForTimerObligation(gomock.Any(), "missing-uuid").Return(nil, nil)
			return func(tt require.TestingT, err error, _ ...interface{}) {
				require.ErrorIs(tt, err, session.ErrTimerObligationNotFound)
			}
		},
		"propagates_resolve_error": func(repo *MockexpireTimerRepoAPI) require.ErrorAssertionFunc {
			repo.EXPECT().ResolveSessionForTimerObligation(gomock.Any(), "missing-uuid").Return(nil, repoErr)
			return func(tt require.TestingT, err error, _ ...interface{}) {
				require.ErrorIs(tt, err, repoErr)
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockexpireTimerRepoAPI(ctrl)
			errAssert := setup(repo)

			m := &Manager{expireTimerRepo: repo}
			_, err := m.ExpireTimer(context.Background(), "missing-uuid")
			errAssert(t, err)
		})
	}
}
