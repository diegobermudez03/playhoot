package accountlogin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/identity"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/accountlogin/internal/repo"
)

func TestHandleCallbackWithoutProviderOrState(t *testing.T) {
	tests := map[string]struct {
		provider string
		state    string
		enabled  bool
	}{
		"provider_not_enabled": {provider: "google", state: "s"},
		"no_state":             {provider: "local", state: "", enabled: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			providers := NewMockproviders(gomock.NewController(t))
			providers.EXPECT().Get(identity.Provider(tc.provider)).Return(nil, tc.enabled)
			manager := &Manager{providers: providers, now: time.Now}

			result, err := manager.HandleCallback(context.Background(), tc.provider, tc.state, "code", "")

			require.NoError(t, err)
			require.Equal(t, identity.LoginCallbackResult{Outcome: identity.LoginFailed}, result)
		})
	}
}

func TestIssueCodeInTx(t *testing.T) {
	type test struct {
		expectedCreated bool
		expectErr       error
	}

	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	external := internalrepo.ExternalIdentity{Issuer: "issuer", Subject: "sub-ana", Email: "ana@example.com", Name: "Ana"}
	issued := func(userID int64) internalrepo.AttemptEvent {
		return internalrepo.AttemptEvent{UserID: userID, ObservedEmail: "ana@example.com", ObservedName: "Ana", OccurredAt: now}
	}

	tests := map[string]func(t *testing.T, repo *MockcallbackRepo) test{
		"bound_identity_resolves_to_its_user": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(3), nil)
			repo.EXPECT().IssueCode(ctx, nil, int64(7), "phl_code", now.Add(60*time.Second), issued(3)).Return(nil)
			return test{}
		},
		"first_sign_in_creates_the_account_without_a_display_name": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(0), nil)
			repo.EXPECT().CreateAccount(ctx, nil, gomock.Any(), external, now).Return(int64(4), true, nil)
			repo.EXPECT().IssueCode(ctx, nil, int64(7), "phl_code", now.Add(60*time.Second), issued(4)).Return(nil)
			return test{expectedCreated: true}
		},
		"bound_by_a_concurrent_sign_in_rolls_back": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(0), nil)
			repo.EXPECT().CreateAccount(ctx, nil, gomock.Any(), external, now).Return(int64(5), false, nil)
			return test{expectErr: errBoundMeanwhile}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			repo := NewMockcallbackRepo(gomock.NewController(t))
			tc := setup(t, repo)
			manager := &Manager{callbackRepo: repo, now: func() time.Time { return now }}

			created, err := manager.issueCodeInTx(ctx, nil, 7, external, "phl_code", now)

			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedCreated, created)
		})
	}
}
