package accountlogin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/refreshtoken"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/accountlogin/internal/repo"
)

func TestCompleteInTx(t *testing.T) {
	type test struct {
		code     string
		verifier string
		replaces string
		expected *signedIn
	}

	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	attempt := func() *internalrepo.CodeAttempt {
		return &internalrepo.CodeAttempt{
			ID: 7, CodeChallenge: challengeOf("verifier"), ExpiresAt: now.Add(30 * time.Second),
			UserID: 3, UserUUID: "user-3",
		}
	}
	// A new account has no display name until the person chooses one.
	ana := identity.User{UUID: "user-3", DisplayName: "", Level: identity.LevelAccount}
	rejected := func(reason string) internalrepo.AttemptEvent {
		return internalrepo.AttemptEvent{Event: internalrepo.EventRejected, Reason: reason, OccurredAt: now}
	}
	completed := internalrepo.AttemptEvent{Event: internalrepo.EventCompleted, UserID: 3, OccurredAt: now}

	tests := map[string]func(t *testing.T, repo *MockcompleteRepo) test{
		"opens_an_account_session_and_ends_the_one_it_replaces": func(t *testing.T, repo *MockcompleteRepo) test {
			repo.EXPECT().LockAttemptByCode(ctx, nil, "phl_code").Return(attempt(), nil)
			repo.EXPECT().OpenSession(ctx, nil, gomock.Any()).DoAndReturn(func(_ context.Context, _ any, session internalrepo.NewSession) error {
				require.Equal(t, int64(3), session.UserID)
				require.Equal(t, "phr_next", session.RefreshToken)
				require.Equal(t, refreshtoken.ExpiresAt(identity.LevelAccount, now, now), session.ExpiresAt)
				return nil
			})
			repo.EXPECT().EndSessionOf(ctx, nil, "phr_guest", now).Return(nil)
			repo.EXPECT().CloseAttempt(ctx, nil, int64(7), completed).Return(nil)
			return test{code: "phl_code", verifier: "verifier", replaces: "phr_guest", expected: &signedIn{user: ana, refreshToken: "phr_next"}}
		},
		"without_a_session_to_replace": func(t *testing.T, repo *MockcompleteRepo) test {
			repo.EXPECT().LockAttemptByCode(ctx, nil, "phl_code").Return(attempt(), nil)
			repo.EXPECT().OpenSession(ctx, nil, gomock.Any()).Return(nil)
			repo.EXPECT().CloseAttempt(ctx, nil, int64(7), completed).Return(nil)
			return test{code: "phl_code", verifier: "verifier", expected: &signedIn{user: ana, refreshToken: "phr_next"}}
		},
		"wrong_verifier_uses_the_code_up": func(t *testing.T, repo *MockcompleteRepo) test {
			repo.EXPECT().LockAttemptByCode(ctx, nil, "phl_code").Return(attempt(), nil)
			repo.EXPECT().CloseAttempt(ctx, nil, int64(7), rejected("verifier_mismatch")).Return(nil)
			return test{code: "phl_code", verifier: "guess"}
		},
		"used_code_is_recorded_as_rejected": func(t *testing.T, repo *MockcompleteRepo) test {
			used := attempt()
			used.CompletedAt = &now
			repo.EXPECT().LockAttemptByCode(ctx, nil, "phl_code").Return(used, nil)
			repo.EXPECT().CloseAttempt(ctx, nil, int64(7), rejected("used")).Return(nil)
			return test{code: "phl_code", verifier: "verifier"}
		},
		"expired_code_is_recorded_as_rejected": func(t *testing.T, repo *MockcompleteRepo) test {
			expired := attempt()
			expired.ExpiresAt = now
			repo.EXPECT().LockAttemptByCode(ctx, nil, "phl_code").Return(expired, nil)
			repo.EXPECT().CloseAttempt(ctx, nil, int64(7), rejected("expired")).Return(nil)
			return test{code: "phl_code", verifier: "verifier"}
		},
		"unknown_code": func(t *testing.T, repo *MockcompleteRepo) test {
			repo.EXPECT().LockAttemptByCode(ctx, nil, "phl_other").Return(nil, nil)
			return test{code: "phl_other", verifier: "verifier"}
		},
		"empty_code": func(t *testing.T, repo *MockcompleteRepo) test {
			return test{verifier: "verifier"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			repo := NewMockcompleteRepo(gomock.NewController(t))
			tc := setup(t, repo)
			manager := &Manager{completeRepo: repo, now: func() time.Time { return now }, newRefresh: func() (string, error) { return "phr_next", nil }}

			got, err := manager.completeInTx(ctx, nil, tc.code, tc.verifier, tc.replaces, now)

			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}
