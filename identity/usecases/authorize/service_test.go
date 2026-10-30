package authorize

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
)

// fakeVerifier accepts exactly the tokens it knows.
type fakeVerifier map[string]identity.User

func (f fakeVerifier) Verify(token string, _ time.Time) (identity.User, error) {
	user, ok := f[token]
	if !ok {
		return identity.User{}, errors.New("invalid")
	}
	return user, nil
}

var (
	guest   = identity.User{UUID: "0f8fad5b-d9cb-469f-a165-70867728950e", DisplayName: "Ana", Level: identity.LevelGuest}
	account = identity.User{UUID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", DisplayName: "Bea", Level: identity.LevelAccount}
)

func TestAuthorize(t *testing.T) {
	type test struct {
		token     string
		operation identity.Operation

		expectedUser     identity.User
		expectedDecision identity.AccessDecision
	}

	tests := map[string]func(t *testing.T) test{
		"guest_can_join_as_player": func(t *testing.T) test {
			return test{token: "guest-jwt", operation: identity.OperationJoinSessionAsPlayer, expectedUser: guest, expectedDecision: identity.AccessAllowed}
		},
		"guest_cannot_create_sessions": func(t *testing.T) test {
			return test{token: "guest-jwt", operation: identity.OperationCreateSession, expectedUser: guest, expectedDecision: identity.AccessForbidden}
		},
		"account_can_create_sessions": func(t *testing.T) test {
			return test{token: "account-jwt", operation: identity.OperationCreateSession, expectedUser: account, expectedDecision: identity.AccessAllowed}
		},
		"invalid_token_is_unauthenticated": func(t *testing.T) test {
			return test{token: "expired-or-forged", operation: identity.OperationJoinSessionAsPlayer, expectedDecision: identity.AccessUnauthenticated}
		},
		"operation_outside_the_catalog_is_forbidden": func(t *testing.T) test {
			return test{token: "account-jwt", operation: "DELETE_EVERYTHING", expectedUser: account, expectedDecision: identity.AccessForbidden}
		},
		"former_test_token_is_unauthenticated": func(t *testing.T) test {
			return test{token: "test-user-1", operation: identity.OperationCreateSession, expectedDecision: identity.AccessUnauthenticated}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			service := &Service{access: fakeVerifier{"guest-jwt": guest, "account-jwt": account}, now: time.Now}

			user, decision, err := service.Authorize(context.Background(), tc.token, tc.operation)

			require.NoError(t, err)
			require.Equal(t, tc.expectedUser, user)
			require.Equal(t, tc.expectedDecision, decision)
		})
	}
}

func TestNewRefusesShortKeys(t *testing.T) {
	_, err := New([]byte("short"))
	require.Error(t, err)
}
