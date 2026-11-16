// Package routing picks the outbound provider for one send from the mode and
// Resend's joint quota.
package routing

import "github.com/diegobermudez03/playhoot/notifications"

// Choice is the provider to try; Wait is set when Resend only has no quota.
type Choice struct {
	Provider notifications.Provider
	Wait     bool
}

// QuotaLeft reports whether Resend's daily and monthly quota both remain.
func QuotaLeft(today, month int) bool {
	return today < notifications.ResendDailyQuota && month < notifications.ResendMonthlyQuota
}

// Choose applies mode: Automatic prefers Resend while quota remains, then
// SES; Resend only waits beyond its quota; SES only is always SES.
func Choose(mode notifications.Mode, resendToday, resendMonth int) Choice {
	switch mode {
	case notifications.ModeSESOnly:
		return Choice{Provider: notifications.ProviderSES}
	case notifications.ModeResendOnly:
		return Choice{Provider: notifications.ProviderResend, Wait: !QuotaLeft(resendToday, resendMonth)}
	}
	if QuotaLeft(resendToday, resendMonth) {
		return Choice{Provider: notifications.ProviderResend}
	}
	return Choice{Provider: notifications.ProviderSES}
}

// FallbackOnQuota reports whether a clear quota refusal by refused may go to
// SES: only Resend's, and only in Automatic mode.
func FallbackOnQuota(mode notifications.Mode, refused notifications.Provider) bool {
	return mode == notifications.ModeAutomatic && refused == notifications.ProviderResend
}
