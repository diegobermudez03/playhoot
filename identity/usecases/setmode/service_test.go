package setmode

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/identity"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/setmode/internal/repo"
)

const testUserUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"

func TestSetInTx(t *testing.T) {
	type test struct {
		expected  switched
		expectErr error
	}

	ctx := context.Background()
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	accountID := int64(7)
	teacher := identity.User{UUID: testUserUUID, DisplayName: "Ana", Level: identity.LevelAccount, Mode: identity.ModeTeacher}

	tests := map[string]func(t *testing.T, accounts *MockaccountStore) test{
		"an_account_switches_to_teaching": func(t *testing.T, accounts *MockaccountStore) test {
			accounts.EXPECT().LockAccount(ctx, nil, testUserUUID).
				Return(&internalrepo.Account{UserID: 4, DisplayName: "Ana", AccountID: &accountID, Mode: "normal"}, nil)
			accounts.EXPECT().ChangeMode(ctx, nil, accountID, int64(4), "normal", "teacher", now).Return(nil)
			return test{expected: switched{user: teacher, outcome: identity.ModeSet, metric: "switched"}}
		},
		"an_account_that_never_switched_is_normal": func(t *testing.T, accounts *MockaccountStore) test {
			accounts.EXPECT().LockAccount(ctx, nil, testUserUUID).
				Return(&internalrepo.Account{UserID: 4, DisplayName: "Ana", AccountID: &accountID}, nil)
			accounts.EXPECT().ChangeMode(ctx, nil, accountID, int64(4), "normal", "teacher", now).Return(nil)
			return test{expected: switched{user: teacher, outcome: identity.ModeSet, metric: "switched"}}
		},
		"asking_for_the_mode_it_already_has_writes_nothing": func(t *testing.T, accounts *MockaccountStore) test {
			accounts.EXPECT().LockAccount(ctx, nil, testUserUUID).
				Return(&internalrepo.Account{UserID: 4, DisplayName: "Ana", AccountID: &accountID, Mode: "teacher"}, nil)
			return test{expected: switched{user: teacher, outcome: identity.ModeSet, metric: "unchanged"}}
		},
		"a_guest_is_refused": func(t *testing.T, accounts *MockaccountStore) test {
			accounts.EXPECT().LockAccount(ctx, nil, testUserUUID).
				Return(&internalrepo.Account{UserID: 4, DisplayName: "Ana"}, nil)
			return test{expected: switched{outcome: identity.ModeNotAccount, metric: string(identity.ModeNotAccount)}}
		},
		"unknown_user": func(t *testing.T, accounts *MockaccountStore) test {
			accounts.EXPECT().LockAccount(ctx, nil, testUserUUID).Return(nil, nil)
			return test{expectErr: identity.ErrUserNotFound}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			accounts := NewMockaccountStore(gomock.NewController(t))
			tc := setup(t, accounts)
			service := &Service{accounts: accounts, now: func() time.Time { return now }}

			got, err := service.setInTx(ctx, nil, testUserUUID, identity.ModeTeacher, now)

			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestSetModeRefusesAnUnknownMode(t *testing.T) {
	result, err := (&Service{now: time.Now}).SetMode(context.Background(), testUserUUID, "student")

	require.NoError(t, err)
	require.Equal(t, identity.SetModeResult{Outcome: identity.ModeInvalid}, result)
}
