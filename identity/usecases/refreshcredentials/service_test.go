package refreshcredentials

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/refreshtoken"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/refreshcredentials/internal/repo"
)

func TestRefreshInTx(t *testing.T) {
	type test struct {
		token string

		expected renewal
	}

	ctx := context.Background()
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { at := now.Add(-d); return &at }
	in := func(d time.Duration) *time.Time { at := now.Add(d); return &at }
	successorID := int64(2)
	guest := identity.User{UUID: "user-1", DisplayName: "Ana", Level: identity.LevelGuest}
	account := identity.User{UUID: "user-2", DisplayName: "Bea", Level: identity.LevelAccount}

	session := func(token string, issuedAgo time.Duration) internalrepo.Session {
		return internalrepo.Session{
			ID: 1, SessionUUID: "session-1", RefreshToken: token, IssuedAt: now.Add(-issuedAgo), StartedAt: now.Add(-issuedAgo),
			UserID: 10, UserUUID: "user-1", DisplayName: "Ana",
		}
	}

	tests := map[string]func(t *testing.T, sessions *MocksessionStore) test{
		"active_token_renews_and_is_kept": func(t *testing.T, sessions *MocksessionStore) test {
			current := session("phr_a", time.Hour)
			sessions.EXPECT().LockSessionByRefreshToken(ctx, nil, "phr_a").Return(&current, nil)
			return test{token: "phr_a", expected: renewal{user: guest, refreshToken: "phr_a", outcome: "renewed"}}
		},
		"old_active_token_rotates": func(t *testing.T, sessions *MocksessionStore) test {
			current := session("phr_a", refreshtoken.RotationAge+time.Minute)
			sessions.EXPECT().LockSessionByRefreshToken(ctx, nil, "phr_a").Return(&current, nil)
			sessions.EXPECT().RotateSession(ctx, nil, current, "phr_next", now, (*time.Time)(nil)).Return(nil)
			return test{token: "phr_a", expected: renewal{user: guest, refreshToken: "phr_next", outcome: "rotated"}}
		},
		"account_rotation_moves_its_expiry_within_the_maximum": func(t *testing.T, sessions *MocksessionStore) test {
			current := session("phr_a", refreshtoken.RotationAge+time.Minute)
			current.HasAccount, current.UserUUID, current.DisplayName = true, "user-2", "Bea"
			current.ExpiresAt = in(time.Hour)
			sessions.EXPECT().LockSessionByRefreshToken(ctx, nil, "phr_a").Return(&current, nil)
			sessions.EXPECT().RotateSession(ctx, nil, current, "phr_next", now, in(refreshtoken.AccountInactivity)).Return(nil)
			return test{token: "phr_a", expected: renewal{user: account, refreshToken: "phr_next", outcome: "rotated"}}
		},
		"expired_active_token_is_invalid": func(t *testing.T, sessions *MocksessionStore) test {
			current := session("phr_a", time.Hour)
			current.ExpiresAt = ago(time.Second)
			sessions.EXPECT().LockSessionByRefreshToken(ctx, nil, "phr_a").Return(&current, nil)
			return test{token: "phr_a", expected: renewal{outcome: "invalid"}}
		},
		"token_just_replaced_gets_the_active_one": func(t *testing.T, sessions *MocksessionStore) test {
			previous := session("phr_old", time.Hour)
			previous.DeactivatedAt, previous.ReplacedByID = ago(time.Minute), &successorID
			active := session("phr_new", time.Minute)
			active.ID = successorID
			sessions.EXPECT().LockSessionByRefreshToken(ctx, nil, "phr_old").Return(&previous, nil)
			sessions.EXPECT().LockSessionByID(ctx, nil, successorID).Return(&active, nil)
			return test{token: "phr_old", expected: renewal{user: guest, refreshToken: "phr_new", outcome: "previous"}}
		},
		"token_replaced_too_long_ago_is_invalid": func(t *testing.T, sessions *MocksessionStore) test {
			previous := session("phr_old", time.Hour)
			previous.DeactivatedAt, previous.ReplacedByID = ago(refreshtoken.PreviousGrace), &successorID
			sessions.EXPECT().LockSessionByRefreshToken(ctx, nil, "phr_old").Return(&previous, nil)
			return test{token: "phr_old", expected: renewal{outcome: "invalid"}}
		},
		"signed_out_token_is_invalid": func(t *testing.T, sessions *MocksessionStore) test {
			ended := session("phr_a", time.Hour)
			ended.DeactivatedAt = ago(time.Second)
			sessions.EXPECT().LockSessionByRefreshToken(ctx, nil, "phr_a").Return(&ended, nil)
			return test{token: "phr_a", expected: renewal{outcome: "invalid"}}
		},
		"previous_token_of_a_signed_out_session_is_invalid": func(t *testing.T, sessions *MocksessionStore) test {
			previous := session("phr_old", time.Hour)
			previous.DeactivatedAt, previous.ReplacedByID = ago(time.Minute), &successorID
			successor := session("phr_new", time.Minute)
			successor.ID, successor.DeactivatedAt = successorID, ago(time.Second)
			sessions.EXPECT().LockSessionByRefreshToken(ctx, nil, "phr_old").Return(&previous, nil)
			sessions.EXPECT().LockSessionByID(ctx, nil, successorID).Return(&successor, nil)
			return test{token: "phr_old", expected: renewal{outcome: "invalid"}}
		},
		"unknown_token_is_invalid": func(t *testing.T, sessions *MocksessionStore) test {
			sessions.EXPECT().LockSessionByRefreshToken(ctx, nil, "phr_x").Return(nil, nil)
			return test{token: "phr_x", expected: renewal{outcome: "invalid"}}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			sessions := NewMocksessionStore(ctrl)
			tc := setup(t, sessions)
			service := &Service{sessions: sessions, now: func() time.Time { return now }, newRefresh: func() (string, error) { return "phr_next", nil }}

			got, err := service.refreshInTx(ctx, nil, tc.token, now)

			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}
