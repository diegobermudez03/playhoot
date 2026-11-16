package render

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/notifications"
)

func TestRender(t *testing.T) {
	links := Links{Open: "https://web/notifications", Unsubscribe: "https://api/v1/email/unsubscribe/tok"}
	spec := func(typ notifications.Type) notifications.TypeSpec {
		s, _ := notifications.Spec(typ)
		return s
	}
	tests := map[string]func(t *testing.T){
		"every type has a subject in every language": func(t *testing.T) {
			for _, lang := range []notifications.Language{notifications.LanguageSpanish, notifications.LanguageEnglish, notifications.LanguagePortuguese} {
				for _, s := range notifications.Matrix {
					require.NotEmpty(t, Render(s, nil, lang, links).Subject, "%s %s", lang, s.Type)
				}
			}
		},
		"skeleton content never enters the email unless allowed": func(t *testing.T) {
			msg := Render(spec(notifications.TypeClassActivity), map[string]any{"grade": "9.5", "student": "Ana"}, notifications.LanguageEnglish, links)
			require.NotContains(t, msg.Text, "9.5")
			require.NotContains(t, msg.Text, "Ana")
			require.Contains(t, msg.Text, links.Open)
		},
		"allowed values are written": func(t *testing.T) {
			msg := Render(spec(notifications.TypeBudgetLevelChanged), map[string]any{"level": "paused", "reason": "secret"}, notifications.LanguageEnglish, links)
			require.Contains(t, msg.Text, "Level: paused")
			require.NotContains(t, msg.Text, "secret")
		},
		"only optional types carry the unsubscribe link": func(t *testing.T) {
			require.Equal(t, links.Unsubscribe, Render(spec(notifications.TypeAIWorkFinished), nil, notifications.LanguageEnglish, links).Unsubscribe)
			msg := Render(spec(notifications.TypeAccountSecurity), nil, notifications.LanguageEnglish, links)
			require.Empty(t, msg.Unsubscribe)
			require.NotContains(t, msg.Text, links.Unsubscribe)
		},
		"unknown language falls back to the default": func(t *testing.T) {
			require.Equal(t, Render(spec(notifications.TypeWelcome), nil, notifications.DefaultLanguage, links), Render(spec(notifications.TypeWelcome), nil, "fr", links))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
