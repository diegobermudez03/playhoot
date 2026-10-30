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
		attempt            internalrepo.AnsweredAttempt
		expectedResolution accountResolution
		expectErr          error
	}

	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	external := internalrepo.ExternalIdentity{Issuer: "issuer", Subject: "sub-ana", Email: "ana@example.com", Name: "Ana"}
	guestID := int64(9)
	issued := func(userID int64, resolution accountResolution) internalrepo.AttemptEvent {
		return internalrepo.AttemptEvent{Reason: string(resolution), UserID: userID, ObservedEmail: "ana@example.com", ObservedName: "Ana", OccurredAt: now}
	}
	plain := internalrepo.AnsweredAttempt{ID: 7}
	fromGuest := internalrepo.AnsweredAttempt{ID: 7, GuestUserID: &guestID}
	expires := now.Add(60 * time.Second)

	tests := map[string]func(t *testing.T, repo *MockcallbackRepo) test{
		"bound_identity_resolves_to_its_user": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(3), nil)
			repo.EXPECT().IssueCode(ctx, nil, int64(7), "phl_code", expires, issued(3, resolvedExisting)).Return(nil)
			return test{attempt: plain, expectedResolution: resolvedExisting}
		},
		"bound_identity_wins_over_the_guest": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(3), nil)
			repo.EXPECT().IssueCode(ctx, nil, int64(7), "phl_code", expires, issued(3, resolvedExisting)).Return(nil)
			return test{attempt: fromGuest, expectedResolution: resolvedExisting}
		},
		"a_new_account_is_the_guest_who_started_the_login": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(0), nil)
			repo.EXPECT().LinkAccount(ctx, nil, guestID, external, now).Return(true, true, nil)
			repo.EXPECT().IssueCode(ctx, nil, int64(7), "phl_code", expires, issued(guestID, resolvedLinkedGuest)).Return(nil)
			return test{attempt: fromGuest, expectedResolution: resolvedLinkedGuest}
		},
		"a_guest_already_an_account_gets_a_new_user": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(0), nil)
			repo.EXPECT().LinkAccount(ctx, nil, guestID, external, now).Return(false, true, nil)
			repo.EXPECT().CreateAccount(ctx, nil, gomock.Any(), external, now).Return(int64(4), true, nil)
			repo.EXPECT().IssueCode(ctx, nil, int64(7), "phl_code", expires, issued(4, resolvedCreated)).Return(nil)
			return test{attempt: fromGuest, expectedResolution: resolvedCreated}
		},
		"linking_an_identity_bound_meanwhile_rolls_back": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(0), nil)
			repo.EXPECT().LinkAccount(ctx, nil, guestID, external, now).Return(true, false, nil)
			return test{attempt: fromGuest, expectErr: errBoundMeanwhile}
		},
		"without_a_guest_a_new_account_without_a_display_name": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(0), nil)
			repo.EXPECT().CreateAccount(ctx, nil, gomock.Any(), external, now).Return(int64(4), true, nil)
			repo.EXPECT().IssueCode(ctx, nil, int64(7), "phl_code", expires, issued(4, resolvedCreated)).Return(nil)
			return test{attempt: plain, expectedResolution: resolvedCreated}
		},
		"creating_an_identity_bound_meanwhile_rolls_back": func(t *testing.T, repo *MockcallbackRepo) test {
			repo.EXPECT().FindBoundUser(ctx, nil, "issuer", "sub-ana").Return(int64(0), nil)
			repo.EXPECT().CreateAccount(ctx, nil, gomock.Any(), external, now).Return(int64(5), false, nil)
			return test{attempt: plain, expectErr: errBoundMeanwhile}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			repo := NewMockcallbackRepo(gomock.NewController(t))
			tc := setup(t, repo)
			manager := &Manager{callbackRepo: repo, now: func() time.Time { return now }}

			resolution, err := manager.issueCodeInTx(ctx, nil, tc.attempt, external, "phl_code", now)

			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedResolution, resolution)
		})
	}
}
