package refreshtoken

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
)

var now = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func TestNew(t *testing.T) {
	first, err := New()
	require.NoError(t, err)
	second, err := New()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(first, "phr_"))
	require.Len(t, first, len("phr_")+43, "256 random bits, unpadded base64url")
	require.NotEqual(t, first, second)
}

func TestExpiresAt(t *testing.T) {
	type test struct {
		level     identity.Level
		issuedAt  time.Time
		startedAt time.Time

		expected *time.Time
	}

	at := func(t time.Time) *time.Time { return &t }

	tests := map[string]func(t *testing.T) test{
		"guest_never_expires": func(t *testing.T) test {
			return test{level: identity.LevelGuest, issuedAt: now, startedAt: now}
		},
		"account_expires_after_its_inactivity_window": func(t *testing.T) test {
			return test{level: identity.LevelAccount, issuedAt: now, startedAt: now, expected: at(now.Add(AccountInactivity))}
		},
		"account_never_outlives_its_maximum": func(t *testing.T) test {
			started := now.Add(-80 * 24 * time.Hour)
			return test{level: identity.LevelAccount, issuedAt: now, startedAt: started, expected: at(started.Add(AccountMaxLifetime))}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			require.Equal(t, tc.expected, ExpiresAt(tc.level, tc.issuedAt, tc.startedAt))
		})
	}
}

func TestTimeRules(t *testing.T) {
	expiry := now.Add(time.Hour)

	require.False(t, Expired(nil, now), "no expiry never expires")
	require.False(t, Expired(&expiry, now))
	require.True(t, Expired(&expiry, expiry))

	require.False(t, ShouldRotate(now.Add(-RotationAge), now), "exactly the rotation age does not rotate yet")
	require.True(t, ShouldRotate(now.Add(-RotationAge-time.Second), now))

	require.True(t, WithinPreviousGrace(now.Add(-PreviousGrace+time.Second), now))
	require.False(t, WithinPreviousGrace(now.Add(-PreviousGrace), now))
}
