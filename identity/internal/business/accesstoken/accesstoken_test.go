package accesstoken

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
)

var (
	key   = []byte("0123456789abcdef0123456789abcdef")
	other = []byte("fedcba9876543210fedcba9876543210")
	user  = identity.User{UUID: "0f8fad5b-d9cb-469f-a165-70867728950e", DisplayName: "Ana", Level: identity.LevelGuest}
	now   = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
)

func TestNewSignerRefusesShortKeys(t *testing.T) {
	_, err := NewSigner([]byte("short"))
	require.Error(t, err)
}

func TestVerify(t *testing.T) {
	type test struct {
		token string
		at    time.Time

		expectedUser identity.User
		expectErr    bool
	}

	signer, err := NewSigner(key)
	require.NoError(t, err)
	token, expiresAt, err := signer.Issue(user, now)
	require.NoError(t, err)
	require.Equal(t, now.Add(Lifetime), expiresAt)
	otherSigner, err := NewSigner(other)
	require.NoError(t, err)
	foreign, _, err := otherSigner.Issue(user, now)
	require.NoError(t, err)

	tests := map[string]func(t *testing.T) test{
		"valid_token_gives_its_user": func(t *testing.T) test {
			return test{token: token, at: now.Add(time.Minute), expectedUser: user}
		},
		"token_from_an_instance_whose_clock_is_slightly_ahead_is_valid": func(t *testing.T) test {
			return test{token: token, at: now.Add(-30 * time.Second), expectedUser: user}
		},
		"token_issued_too_far_in_the_future_is_invalid": func(t *testing.T) test {
			return test{token: token, at: now.Add(-2 * ClockSkew), expectErr: true}
		},
		"expired_token_is_invalid": func(t *testing.T) test {
			return test{token: token, at: expiresAt, expectErr: true}
		},
		"token_signed_with_another_key_is_invalid": func(t *testing.T) test {
			return test{token: foreign, at: now, expectErr: true}
		},
		"tampered_payload_is_invalid": func(t *testing.T) test {
			parts := strings.Split(token, ".")
			parts[1] = parts[1][:len(parts[1])-2] + "AA"
			return test{token: strings.Join(parts, "."), at: now, expectErr: true}
		},
		"unsigned_token_is_invalid": func(t *testing.T) test {
			parts := strings.Split(token, ".")
			// {"alg":"none"} as the header, no signature.
			return test{token: "eyJhbGciOiJub25lIn0." + parts[1] + ".", at: now, expectErr: true}
		},
		"garbage_is_invalid": func(t *testing.T) test {
			return test{token: "test-user-1", at: now, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			got, err := signer.Verify(tc.token, tc.at)

			if tc.expectErr {
				require.ErrorIs(t, err, ErrInvalid)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedUser, got)
		})
	}
}
