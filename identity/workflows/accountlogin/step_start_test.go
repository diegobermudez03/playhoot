package accountlogin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/loginproviders"
)

func TestStartLoginRefusals(t *testing.T) {
	local := loginproviders.NewLocal(loginproviders.LocalConfig{AuthorizeURL: "/authorize", CallbackURL: "/callback"})

	tests := map[string]struct {
		provider  string
		challenge string
		found     bool
		expected  identity.StartLoginOutcome
	}{
		"provider_not_enabled":        {provider: "google", challenge: challengeOf("verifier"), found: false, expected: identity.LoginProviderUnavailable},
		"challenge_that_is_not_s256":  {provider: "local", challenge: "verifier", found: true, expected: identity.LoginInvalidCodeChallenge},
		"empty_challenge_is_not_s256": {provider: "local", challenge: "", found: true, expected: identity.LoginInvalidCodeChallenge},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			providers := NewMockproviders(gomock.NewController(t))
			if tc.found {
				providers.EXPECT().Get(identity.Provider(tc.provider)).Return(local, true)
			} else {
				providers.EXPECT().Get(identity.Provider(tc.provider)).Return(nil, false)
			}
			manager := &Manager{providers: providers, now: time.Now}

			result, err := manager.StartLogin(context.Background(), tc.provider, tc.challenge)

			require.NoError(t, err)
			require.Equal(t, identity.StartLoginResult{Outcome: tc.expected}, result)
		})
	}
}
