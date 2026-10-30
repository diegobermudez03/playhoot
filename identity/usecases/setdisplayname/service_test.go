package setdisplayname

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/identity"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/setdisplayname/internal/repo"
)

func TestSetInTx(t *testing.T) {
	type test struct {
		expected  identity.SetDisplayNameResult
		expectErr error
	}

	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	old := "Ana"
	named := func(level identity.Level) identity.SetDisplayNameResult {
		return identity.SetDisplayNameResult{Outcome: identity.DisplayNameSet, User: identity.User{UUID: "user-1", DisplayName: "Bea", Level: level}}
	}

	tests := map[string]func(t *testing.T, users *MockuserStore) test{
		"an_account_chooses_its_first_name": func(t *testing.T, users *MockuserStore) test {
			users.EXPECT().LockUser(ctx, nil, "user-1").Return(&internalrepo.User{ID: 4, HasAccount: true}, nil)
			users.EXPECT().ChangeDisplayName(ctx, nil, int64(4), (*string)(nil), "Bea", now).Return(nil)
			return test{expected: named(identity.LevelAccount)}
		},
		"a_guest_changes_its_name_keeping_the_old_one": func(t *testing.T, users *MockuserStore) test {
			users.EXPECT().LockUser(ctx, nil, "user-1").Return(&internalrepo.User{ID: 4, DisplayName: &old}, nil)
			users.EXPECT().ChangeDisplayName(ctx, nil, int64(4), &old, "Bea", now).Return(nil)
			return test{expected: named(identity.LevelGuest)}
		},
		"the_same_name_changes_nothing": func(t *testing.T, users *MockuserStore) test {
			same := "Bea"
			users.EXPECT().LockUser(ctx, nil, "user-1").Return(&internalrepo.User{ID: 4, DisplayName: &same, HasAccount: true}, nil)
			return test{expected: named(identity.LevelAccount)}
		},
		"unknown_user": func(t *testing.T, users *MockuserStore) test {
			users.EXPECT().LockUser(ctx, nil, "user-1").Return(nil, nil)
			return test{expectErr: identity.ErrUserNotFound}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			users := NewMockuserStore(gomock.NewController(t))
			tc := setup(t, users)
			service := &Service{users: users, now: func() time.Time { return now }}

			got, err := service.setInTx(ctx, nil, "user-1", "Bea", now)

			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestSetDisplayNameRefusesAnInvalidName(t *testing.T) {
	result, err := (&Service{now: time.Now}).SetDisplayName(context.Background(), "user-1", "   ")

	require.NoError(t, err)
	require.Equal(t, identity.SetDisplayNameResult{Outcome: identity.DisplayNameInvalid}, result)
}
