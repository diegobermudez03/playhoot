package createguest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/createguest/internal/repo"
)

func TestCreateGuest(t *testing.T) {
	type svcMocks struct {
		guests *MockguestStore
		access *MockaccessIssuer
	}
	type test struct {
		displayName string

		expected  identity.CreateGuestResult
		expectErr require.ErrorAssertionFunc
		// check replaces comparing with expected, for results holding new tokens.
		check func(t *testing.T, result identity.CreateGuestResult)
	}

	const key = "4f6c8a2e-1b3d-4e5f-9a7b-0c1d2e3f4a5b"
	ctx := context.Background()
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	accessExpiry := now.Add(15 * time.Minute)
	active := "phr_active"

	credentialsFor := func(userUUID, refresh string) identity.Credentials {
		return identity.Credentials{
			User:            identity.User{UUID: userUUID, DisplayName: "Ana", Level: identity.LevelGuest},
			AccessToken:     "access-jwt",
			AccessIssuedAt:  now,
			AccessExpiresAt: accessExpiry,
			RefreshToken:    refresh,
		}
	}

	tests := map[string]func(t *testing.T, m svcMocks) test{
		"creates_a_guest": func(t *testing.T, m svcMocks) test {
			var created internalrepo.NewGuest
			m.guests.EXPECT().ClaimGuestRequest(ctx, nil, gomock.Any()).DoAndReturn(func(_ context.Context, _ *gorm.DB, guest internalrepo.NewGuest) (int64, *internalrepo.GuestRequest, error) {
				created = guest
				return 1, nil, nil
			})
			m.guests.EXPECT().CreateGuestUser(ctx, nil, gomock.Any()).DoAndReturn(func(_ context.Context, _ *gorm.DB, guest internalrepo.NewGuest) error {
				require.Equal(t, created, guest)
				return nil
			})
			m.access.EXPECT().Issue(gomock.Any(), now).DoAndReturn(func(user identity.User, _ time.Time) (string, time.Time, error) {
				require.Equal(t, identity.User{UUID: created.UserUUID, DisplayName: "Ana", Level: identity.LevelGuest}, user)
				return "access-jwt", accessExpiry, nil
			})
			t.Cleanup(func() {
				require.Equal(t, key, created.IdempotencyKey)
				require.Equal(t, "Ana", created.DisplayName)
				require.Equal(t, now, created.RequestedAt)
				require.NotEmpty(t, created.SessionUUID)
			})
			return test{
				displayName: "Ana",
				expectErr:   require.NoError,
				check: func(t *testing.T, result identity.CreateGuestResult) {
					require.Equal(t, identity.GuestCreated, result.Outcome)
					require.Equal(t, "access-jwt", result.Credentials.AccessToken)
					require.Equal(t, created.RefreshToken, result.Credentials.RefreshToken)
					require.Contains(t, result.Credentials.RefreshToken, "phr_")
				},
			}
		},
		"same_key_and_name_soon_after_gets_the_same_guest": func(t *testing.T, m svcMocks) test {
			m.guests.EXPECT().ClaimGuestRequest(ctx, nil, gomock.Any()).Return(int64(0), &internalrepo.GuestRequest{
				DisplayName: "Ana", UserUUID: "user-1", RequestedAt: now.Add(-time.Minute), ActiveRefreshToken: &active,
			}, nil)
			m.access.EXPECT().Issue(identity.User{UUID: "user-1", DisplayName: "Ana", Level: identity.LevelGuest}, now).Return("access-jwt", accessExpiry, nil)
			return test{
				displayName: "Ana",
				expected:    identity.CreateGuestResult{Outcome: identity.GuestCreated, Credentials: credentialsFor("user-1", active)},
				expectErr:   require.NoError,
			}
		},
		"same_key_with_another_name_conflicts": func(t *testing.T, m svcMocks) test {
			m.guests.EXPECT().ClaimGuestRequest(ctx, nil, gomock.Any()).Return(int64(0), &internalrepo.GuestRequest{
				DisplayName: "Bea", UserUUID: "user-1", RequestedAt: now, ActiveRefreshToken: &active,
			}, nil)
			return test{
				displayName: "Ana",
				expectErr: func(t require.TestingT, err error, _ ...any) {
					require.ErrorIs(t, err, identity.ErrIdempotencyConflict)
				},
			}
		},
		"same_key_after_the_window_is_spent": func(t *testing.T, m svcMocks) test {
			m.guests.EXPECT().ClaimGuestRequest(ctx, nil, gomock.Any()).Return(int64(0), &internalrepo.GuestRequest{
				DisplayName: "Ana", UserUUID: "user-1", RequestedAt: now.Add(-ReplayWindow), ActiveRefreshToken: &active,
			}, nil)
			return test{
				displayName: "Ana",
				expected:    identity.CreateGuestResult{Outcome: identity.GuestIdempotencyKeySpent},
				expectErr:   require.NoError,
			}
		},
		"same_key_after_signing_out_is_spent": func(t *testing.T, m svcMocks) test {
			m.guests.EXPECT().ClaimGuestRequest(ctx, nil, gomock.Any()).Return(int64(0), &internalrepo.GuestRequest{
				DisplayName: "Ana", UserUUID: "user-1", RequestedAt: now,
			}, nil)
			return test{
				displayName: "Ana",
				expected:    identity.CreateGuestResult{Outcome: identity.GuestIdempotencyKeySpent},
				expectErr:   require.NoError,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := svcMocks{guests: NewMockguestStore(ctrl), access: NewMockaccessIssuer(ctrl)}
			tc := setup(t, m)
			service := &Service{guests: m.guests, access: m.access, now: func() time.Time { return now }}

			result, err := service.createInTx(ctx, nil, tc.displayName, key, now)

			tc.expectErr(t, err)
			if tc.check != nil {
				tc.check(t, result)
				return
			}
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestCreateGuestRefusesAnInvalidName(t *testing.T) {
	service := &Service{now: time.Now}

	result, err := service.CreateGuest(context.Background(), "   ", "4f6c8a2e-1b3d-4e5f-9a7b-0c1d2e3f4a5b")

	require.NoError(t, err)
	require.Equal(t, identity.CreateGuestResult{Outcome: identity.GuestInvalidDisplayName}, result)
}
