package endsession

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestEndSession(t *testing.T) {
	type test struct {
		expectErr bool
	}

	ctx := context.Background()
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, sessions *MocksessionStore) test{
		"ends_the_active_session": func(t *testing.T, sessions *MocksessionStore) test {
			sessions.EXPECT().DeactivateSessionOf(ctx, "phr_token", now).Return(true, nil)
			return test{}
		},
		"nothing_to_end_is_not_an_error": func(t *testing.T, sessions *MocksessionStore) test {
			sessions.EXPECT().DeactivateSessionOf(ctx, "phr_token", now).Return(false, nil)
			return test{}
		},
		"storage_failure_is_returned": func(t *testing.T, sessions *MocksessionStore) test {
			sessions.EXPECT().DeactivateSessionOf(ctx, "phr_token", now).Return(false, errors.New("down"))
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			sessions := NewMocksessionStore(ctrl)
			tc := setup(t, sessions)
			svc := &Service{sessions: sessions, now: func() time.Time { return now }}

			err := svc.EndSession(ctx, "phr_token")

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
