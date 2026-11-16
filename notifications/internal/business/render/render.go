// Package render writes an entry's email: minimal plain text in the
// recipient's language, a link to the protected content, the unsubscribe
// link for optional types and nothing that tracks opens or clicks. Only the
// skeleton values the type allows ever enter an email.
package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/diegobermudez03/playhoot/notifications"
)

// Links are the addresses an email may point to.
type Links struct {
	// Open leads to the notification inside Playhoot.
	Open string
	// Unsubscribe is the one-click link, used for optional types only.
	Unsubscribe string
}

// Message is a rendered email.
type Message struct {
	Subject     string
	Text        string
	Unsubscribe string
}

type phrases struct {
	subjects    map[notifications.Type]string
	intro       string
	open        string
	unsubscribe string
	values      map[string]string
}

var catalog = map[notifications.Language]phrases{
	notifications.LanguageSpanish: {
		subjects: map[notifications.Type]string{
			notifications.TypeAccessCode:           "Tu código de acceso",
			notifications.TypeWelcome:              "Te damos la bienvenida a Playhoot",
			notifications.TypePurchaseConfirmed:    "Compra confirmada",
			notifications.TypePlanChanged:          "Cambio en tu plan",
			notifications.TypePaymentFailed:        "Problema con tu pago",
			notifications.TypeTrialUpdate:          "Novedades de tu prueba",
			notifications.TypeAccountSecurity:      "Aviso de seguridad de tu cuenta",
			notifications.TypeGuardianAuthorized:   "Autorización registrada",
			notifications.TypeAIWorkFinished:       "Tu trabajo con IA cambió de estado",
			notifications.TypeCapacityWarning:      "Tus capacidades están por agotarse",
			notifications.TypeDiscoveryReview:      "Resultado de la revisión para Discovery",
			notifications.TypeDiscoveryRestriction: "Restricción en Discovery",
			notifications.TypeClassActivity:        "Novedades de tu clase",
			notifications.TypeMarketing:            "Novedades de Playhoot",
			notifications.TypeBudgetLevelChanged:   "Cambio en el control de gasto",
		},
		intro:       "Tienes un aviso nuevo en Playhoot.",
		open:        "Ábrelo aquí",
		unsubscribe: "Dejar de recibir estos correos",
		values:      map[string]string{"code": "Código", "level": "Nivel"},
	},
	notifications.LanguageEnglish: {
		subjects: map[notifications.Type]string{
			notifications.TypeAccessCode:           "Your access code",
			notifications.TypeWelcome:              "Welcome to Playhoot",
			notifications.TypePurchaseConfirmed:    "Purchase confirmed",
			notifications.TypePlanChanged:          "A change to your plan",
			notifications.TypePaymentFailed:        "A problem with your payment",
			notifications.TypeTrialUpdate:          "News about your trial",
			notifications.TypeAccountSecurity:      "Security notice for your account",
			notifications.TypeGuardianAuthorized:   "Authorization recorded",
			notifications.TypeAIWorkFinished:       "Your AI work changed state",
			notifications.TypeCapacityWarning:      "Your capacities are running out",
			notifications.TypeDiscoveryReview:      "Discovery review result",
			notifications.TypeDiscoveryRestriction: "Discovery restriction",
			notifications.TypeClassActivity:        "News from your class",
			notifications.TypeMarketing:            "News from Playhoot",
			notifications.TypeBudgetLevelChanged:   "Spending control changed",
		},
		intro:       "You have a new notification in Playhoot.",
		open:        "Open it here",
		unsubscribe: "Stop receiving these emails",
		values:      map[string]string{"code": "Code", "level": "Level"},
	},
	notifications.LanguagePortuguese: {
		subjects: map[notifications.Type]string{
			notifications.TypeAccessCode:           "Seu código de acesso",
			notifications.TypeWelcome:              "Boas-vindas ao Playhoot",
			notifications.TypePurchaseConfirmed:    "Compra confirmada",
			notifications.TypePlanChanged:          "Mudança no seu plano",
			notifications.TypePaymentFailed:        "Problema com seu pagamento",
			notifications.TypeTrialUpdate:          "Novidades do seu teste",
			notifications.TypeAccountSecurity:      "Aviso de segurança da sua conta",
			notifications.TypeGuardianAuthorized:   "Autorização registrada",
			notifications.TypeAIWorkFinished:       "Seu trabalho com IA mudou de estado",
			notifications.TypeCapacityWarning:      "Suas capacidades estão acabando",
			notifications.TypeDiscoveryReview:      "Resultado da revisão para o Discovery",
			notifications.TypeDiscoveryRestriction: "Restrição no Discovery",
			notifications.TypeClassActivity:        "Novidades da sua turma",
			notifications.TypeMarketing:            "Novidades do Playhoot",
			notifications.TypeBudgetLevelChanged:   "Mudança no controle de gastos",
		},
		intro:       "Você tem um novo aviso no Playhoot.",
		open:        "Abra aqui",
		unsubscribe: "Deixar de receber estes e-mails",
		values:      map[string]string{"code": "Código", "level": "Nível"},
	},
}

// Supported reports whether lang has a catalog.
func Supported(lang notifications.Language) bool {
	_, ok := catalog[lang]
	return ok
}

// Render writes spec's email in lang (the default language when unknown).
// The open link is left out for a type not shown inside Playhoot.
func Render(spec notifications.TypeSpec, skeleton map[string]any, lang notifications.Language, links Links) Message {
	p, ok := catalog[lang]
	if !ok {
		p = catalog[notifications.DefaultLanguage]
	}
	lines := []string{p.intro}
	allowed := append([]string(nil), spec.EmailValues...)
	sort.Strings(allowed)
	for _, key := range allowed {
		if value, ok := skeleton[key]; ok {
			lines = append(lines, fmt.Sprintf("%s: %v", p.values[key], value))
		}
	}
	if spec.InApp && links.Open != "" {
		lines = append(lines, "", p.open+": "+links.Open)
	}
	msg := Message{Subject: p.subjects[spec.Type]}
	if spec.Email == notifications.EmailOptional && links.Unsubscribe != "" {
		lines = append(lines, "", p.unsubscribe+": "+links.Unsubscribe)
		msg.Unsubscribe = links.Unsubscribe
	}
	msg.Text = strings.Join(lines, "\n") + "\n"
	return msg
}
