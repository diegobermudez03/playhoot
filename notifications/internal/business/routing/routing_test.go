package routing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/notifications"
)

func TestChoose(t *testing.T) {
	tests := map[string]func(t *testing.T){
		"automatic uses Resend while both quotas remain": func(t *testing.T) {
			require.Equal(t, Choice{Provider: notifications.ProviderResend}, Choose(notifications.ModeAutomatic, 99, 2999))
		},
		"automatic switches to SES when either quota is spent": func(t *testing.T) {
			require.Equal(t, Choice{Provider: notifications.ProviderSES}, Choose(notifications.ModeAutomatic, 100, 10))
			require.Equal(t, Choice{Provider: notifications.ProviderSES}, Choose(notifications.ModeAutomatic, 0, 3000))
		},
		"resend only waits beyond its quota": func(t *testing.T) {
			require.Equal(t, Choice{Provider: notifications.ProviderResend, Wait: true}, Choose(notifications.ModeResendOnly, 100, 100))
			require.Equal(t, Choice{Provider: notifications.ProviderResend}, Choose(notifications.ModeResendOnly, 1, 1))
		},
		"ses only ignores quota": func(t *testing.T) {
			require.Equal(t, Choice{Provider: notifications.ProviderSES}, Choose(notifications.ModeSESOnly, 0, 0))
		},
		"only automatic falls back after a Resend quota refusal": func(t *testing.T) {
			require.True(t, FallbackOnQuota(notifications.ModeAutomatic, notifications.ProviderResend))
			require.False(t, FallbackOnQuota(notifications.ModeResendOnly, notifications.ProviderResend))
			require.False(t, FallbackOnQuota(notifications.ModeAutomatic, notifications.ProviderSES))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
