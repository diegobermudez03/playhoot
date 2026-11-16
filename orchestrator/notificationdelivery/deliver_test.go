package notificationdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/notifications"
)

type fakeContacts struct {
	admin    string
	contacts map[string]identity.Contact
}

func (f fakeContacts) ReadContact(_ context.Context, user string) (identity.Contact, error) {
	return f.contacts[user], nil
}

func (f fakeContacts) AdminUserUUID(context.Context) (string, bool, error) {
	return f.admin, f.admin != "", nil
}

type fakeAcceptances struct{ consent bool }

func (f fakeAcceptances) HasCurrentAcceptance(context.Context, string, compliance.TextKind) (bool, error) {
	return f.consent, nil
}

type fakeEntries struct{ got []notifications.Notice }

func (f *fakeEntries) AcceptEntry(_ context.Context, n notifications.Notice) (notifications.Acceptance, error) {
	f.got = append(f.got, n)
	if _, ok := notifications.Spec(n.Type); !ok {
		return notifications.Acceptance{Outcome: notifications.AcceptInvalid}, nil
	}
	return notifications.Acceptance{Outcome: notifications.Accepted}, nil
}

func payload(t *testing.T, p Payload) json.RawMessage {
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	return raw
}

func TestDeliver(t *testing.T) {
	ctx := context.Background()
	const admin, user = "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"
	contacts := fakeContacts{admin: admin, contacts: map[string]identity.Contact{
		admin: {Found: true, Account: true, Email: "admin@x.com"},
		user:  {Found: true},
	}}
	tests := map[string]func(t *testing.T){
		"admin recipients resolve to the administrator with their address": func(t *testing.T) {
			entries := &fakeEntries{}
			err := New(contacts, fakeAcceptances{}, entries).Deliver(ctx, "budget", payload(t, Payload{Recipient: RecipientAdmin, Type: string(notifications.TypeBudgetLevelChanged), IdempotencyKey: "k"}))
			require.NoError(t, err)
			require.Equal(t, admin, entries.got[0].Recipient)
			require.Equal(t, "budget", entries.got[0].SourceDomain)
			require.True(t, entries.got[0].HasEmail)
		},
		"no administrator retries": func(t *testing.T) {
			err := New(fakeContacts{}, fakeAcceptances{}, &fakeEntries{}).Deliver(ctx, "budget", payload(t, Payload{Recipient: RecipientAdmin, Type: "x"}))
			require.ErrorIs(t, err, ErrNoAdmin)
		},
		"a guest has no email and marketing asks for consent": func(t *testing.T) {
			entries := &fakeEntries{}
			err := New(contacts, fakeAcceptances{consent: true}, entries).Deliver(ctx, "billing", payload(t, Payload{Recipient: user, Type: string(notifications.TypeMarketing), IdempotencyKey: "k"}))
			require.NoError(t, err)
			require.False(t, entries.got[0].HasEmail)
			require.True(t, entries.got[0].MarketingConsent)
		},
		"unknown recipients and invalid notices turn dead": func(t *testing.T) {
			d := New(contacts, fakeAcceptances{}, &fakeEntries{})
			err := d.Deliver(ctx, "budget", payload(t, Payload{Recipient: "33333333-3333-3333-3333-333333333333", Type: string(notifications.TypeWelcome)}))
			require.Error(t, err)
			err = d.Deliver(ctx, "budget", payload(t, Payload{Recipient: user, Type: "push"}))
			require.Error(t, err)
			require.Error(t, d.Deliver(ctx, "budget", json.RawMessage(`{`)))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

type fakeBudget struct {
	admit    budget.Admission
	requests []budget.WorkRequest
	settled  []string
}

func (f *fakeBudget) AdmitWork(_ context.Context, r budget.WorkRequest) (budget.Admission, error) {
	f.requests = append(f.requests, r)
	return f.admit, nil
}

func (f *fakeBudget) SettleWork(_ context.Context, id string, _ budget.Settlement) (budget.SettleOutcome, error) {
	f.settled = append(f.settled, id)
	return budget.Settled, nil
}

type fakeSender struct {
	outcome notifications.SendOutcome
	calls   []notifications.Recipient
}

func (f *fakeSender) SendEmail(_ context.Context, _ string, to notifications.Recipient) (notifications.SendOutcome, error) {
	f.calls = append(f.calls, to)
	return f.outcome, nil
}

func TestEmail(t *testing.T) {
	ctx := context.Background()
	const user = "22222222-2222-2222-2222-222222222222"
	contacts := fakeContacts{contacts: map[string]identity.Contact{user: {Found: true, Account: true, Email: "a@x.com"}}}
	task := func(necessary bool) json.RawMessage {
		raw, _ := json.Marshal(notifications.SendTask{EntryUUID: "e", Recipient: user, Necessary: necessary})
		return raw
	}
	tests := map[string]func(t *testing.T){
		"an admitted email is sent and its reservation released": func(t *testing.T) {
			b := &fakeBudget{admit: budget.Admission{Outcome: budget.Admitted, ReservationUUID: "r"}}
			s := &fakeSender{outcome: notifications.EmailSent}
			require.NoError(t, NewEmail(contacts, b, s, b).Run(ctx, task(true)))
			require.Equal(t, []notifications.Recipient{{Email: "a@x.com", Language: notifications.DefaultLanguage}}, s.calls)
			require.Equal(t, []string{"r"}, b.settled)
			require.False(t, b.requests[0].Discretionary, "necessary email goes first")
		},
		"a budget refusal pauses without sending": func(t *testing.T) {
			b := &fakeBudget{admit: budget.Admission{Outcome: budget.Refused, Reason: budget.RefusedRestricted}}
			s := &fakeSender{}
			err := NewEmail(contacts, b, s, b).Run(ctx, task(false))
			require.ErrorIs(t, err, ErrEmailPaused)
			require.Empty(t, s.calls)
			require.True(t, b.requests[0].Discretionary)
		},
		"a waiting send retries after releasing": func(t *testing.T) {
			b := &fakeBudget{admit: budget.Admission{Outcome: budget.Admitted, ReservationUUID: "r"}}
			err := NewEmail(contacts, b, &fakeSender{outcome: notifications.EmailRetry}, b).Run(ctx, task(true))
			require.True(t, errors.Is(err, ErrEmailPaused))
			require.Equal(t, []string{"r"}, b.settled)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
