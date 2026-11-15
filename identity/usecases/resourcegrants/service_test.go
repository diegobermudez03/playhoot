package resourcegrants

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/resourcegrant"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/resourcegrants/internal/repo"
)

func TestClaimResourceGrant(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	token, err := resourcegrant.NewToken()
	require.NoError(t, err)
	hash := resourcegrant.Hash(token)
	used := now.Add(-time.Minute)

	cases := map[string]struct {
		setup    func(store *MockgrantStore)
		token    string
		expected identity.ClaimedResourceGrant
	}{
		"a_live_grant_is_claimed": {
			setup: func(store *MockgrantStore) {
				store.EXPECT().UseGrant(ctx, hash, now).Return(&internalrepo.Grant{UUID: "g", UserUUID: "u", Kind: "game", ResourceID: "r", Scope: "s"}, nil)
			},
			token:    token,
			expected: identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantClaimed, GrantUUID: "g", UserUUID: "u", Kind: "game", ResourceID: "r", Scope: "s"},
		},
		"a_used_grant_says_so": {
			setup: func(store *MockgrantStore) {
				store.EXPECT().UseGrant(ctx, hash, now).Return(nil, nil)
				store.EXPECT().FindGrantByHash(ctx, hash).Return(&internalrepo.Grant{UUID: "g", UsedAt: &used}, nil)
			},
			token:    token,
			expected: identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantUsed},
		},
		"an_unused_unclaimable_grant_expired": {
			setup: func(store *MockgrantStore) {
				store.EXPECT().UseGrant(ctx, hash, now).Return(nil, nil)
				store.EXPECT().FindGrantByHash(ctx, hash).Return(&internalrepo.Grant{UUID: "g"}, nil)
			},
			token:    token,
			expected: identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantExpired},
		},
		"an_unknown_token": {
			setup: func(store *MockgrantStore) {
				store.EXPECT().UseGrant(ctx, hash, now).Return(nil, nil)
				store.EXPECT().FindGrantByHash(ctx, hash).Return(nil, nil)
			},
			token:    token,
			expected: identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantUnknown},
		},
		"a_malformed_token_is_never_looked_up": {
			setup:    func(store *MockgrantStore) {},
			token:    "phl_not-a-grant",
			expected: identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantUnknown},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := NewMockgrantStore(gomock.NewController(t))
			tc.setup(store)
			service := &Service{grants: store, now: func() time.Time { return now }}

			claimed, err := service.ClaimResourceGrant(ctx, tc.token)

			require.NoError(t, err)
			require.Equal(t, tc.expected, claimed)
		})
	}
}
