package channels

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/notifications"
)

func spec(t *testing.T, typ notifications.Type) notifications.TypeSpec {
	t.Helper()
	s, ok := notifications.Spec(typ)
	require.True(t, ok)
	return s
}

func TestDecide(t *testing.T) {
	allOff := notifications.Preferences{}
	allOn := notifications.Preferences{AI: true, Capacity: true, Review: true, Marketing: true}
	tests := map[string]func(t *testing.T){
		"necessary types email even with every preference off": func(t *testing.T) {
			for _, typ := range []notifications.Type{notifications.TypeAccountSecurity, notifications.TypePaymentFailed, notifications.TypeDiscoveryRestriction} {
				require.Equal(t, Decision{InApp: true, Email: true}, Decide(spec(t, typ), true, false, false, allOff), typ)
			}
		},
		"optional types follow their own group only": func(t *testing.T) {
			ai := spec(t, notifications.TypeAIWorkFinished)
			require.False(t, Decide(ai, true, false, false, allOff).Email)
			require.False(t, Decide(ai, true, false, false, notifications.Preferences{Capacity: true, Review: true}).Email)
			require.True(t, Decide(ai, true, false, false, notifications.Preferences{AI: true}).Email)
			require.True(t, Decide(ai, true, false, false, allOff).InApp, "preferences never trim the in-app entry")
		},
		"defaults keep AI and capacity off and review on": func(t *testing.T) {
			require.False(t, Decide(spec(t, notifications.TypeAIWorkFinished), true, false, false, notifications.DefaultPreferences).Email)
			require.False(t, Decide(spec(t, notifications.TypeCapacityWarning), true, false, false, notifications.DefaultPreferences).Email)
			require.True(t, Decide(spec(t, notifications.TypeDiscoveryReview), true, false, false, notifications.DefaultPreferences).Email)
		},
		"marketing needs the preference and the consent": func(t *testing.T) {
			m := spec(t, notifications.TypeMarketing)
			require.False(t, Decide(m, true, false, false, allOn).Email)
			require.False(t, Decide(m, true, false, true, allOff).Email)
			require.True(t, Decide(m, true, false, true, allOn).Email)
		},
		"students never get class email": func(t *testing.T) {
			c := spec(t, notifications.TypeClassActivity)
			require.Equal(t, Decision{InApp: true}, Decide(c, true, true, false, allOn))
			require.True(t, Decide(c, true, false, false, allOff).Email)
		},
		"no address means no email": func(t *testing.T) {
			require.Equal(t, Decision{InApp: true}, Decide(spec(t, notifications.TypeAccountSecurity), false, false, false, allOn))
		},
		"access codes are never an in-app entry": func(t *testing.T) {
			require.Equal(t, Decision{Email: true}, Decide(spec(t, notifications.TypeAccessCode), true, false, false, allOff))
		},
		"optional rules and groups go together": func(t *testing.T) {
			for _, s := range notifications.Matrix {
				require.Equal(t, s.Email == notifications.EmailOptional, s.Group != "", s.Type)
			}
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
