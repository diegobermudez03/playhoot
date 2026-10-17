package authorize

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
)

func TestAuthorize(t *testing.T) {
	type test struct {
		token     string
		operation identity.Operation

		expectedUserUUID string
		expectedDecision identity.AccessDecision
	}

	tests := map[string]func(t *testing.T) test{
		"test_user_is_allowed": func(t *testing.T) test {
			return test{
				token: "test-user-1", operation: identity.OperationCreateSession,
				expectedUserUUID: "aaaaaaaa-aaaa-4aaa-8aaa-000000000001", expectedDecision: identity.AccessAllowed,
			}
		},
		"each_test_token_is_its_own_user": func(t *testing.T) test {
			return test{
				token: "test-user-2", operation: identity.OperationCreateSession,
				expectedUserUUID: "aaaaaaaa-aaaa-4aaa-8aaa-000000000002", expectedDecision: identity.AccessAllowed,
			}
		},
		"guest_cannot_create_sessions": func(t *testing.T) test {
			return test{
				token: "test-guest", operation: identity.OperationCreateSession,
				expectedUserUUID: "aaaaaaaa-aaaa-4aaa-8aaa-0000000000ff", expectedDecision: identity.AccessForbidden,
			}
		},
		"guest_cannot_join_sessions_as_admin": func(t *testing.T) test {
			return test{
				token: "test-guest", operation: identity.OperationJoinSessionAsAdmin,
				expectedUserUUID: "aaaaaaaa-aaaa-4aaa-8aaa-0000000000ff", expectedDecision: identity.AccessForbidden,
			}
		},
		"test_user_can_join_sessions_as_admin": func(t *testing.T) test {
			return test{
				token: "test-user-1", operation: identity.OperationJoinSessionAsAdmin,
				expectedUserUUID: "aaaaaaaa-aaaa-4aaa-8aaa-000000000001", expectedDecision: identity.AccessAllowed,
			}
		},
		"guest_can_join_sessions_as_player": func(t *testing.T) test {
			return test{
				token: "test-guest", operation: identity.OperationJoinSessionAsPlayer,
				expectedUserUUID: "aaaaaaaa-aaaa-4aaa-8aaa-0000000000ff", expectedDecision: identity.AccessAllowed,
			}
		},
		"unknown_token_is_unauthenticated": func(t *testing.T) test {
			return test{token: "anything-else", operation: identity.OperationCreateSession, expectedDecision: identity.AccessUnauthenticated}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			userUUID, decision, err := New().Authorize(context.Background(), tc.token, tc.operation)

			require.NoError(t, err)
			require.Equal(t, tc.expectedUserUUID, userUUID)
			require.Equal(t, tc.expectedDecision, decision)
		})
	}
}
