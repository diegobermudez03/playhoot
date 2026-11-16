// Package notificationdelivery carries notify tasks from every domain's queue
// into Notifications (flow deliver-notification) and runs each entry's email
// task within a budget admission (flow send-notification-email).
package notificationdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// RecipientAdmin addresses a notify task to the administrator.
const RecipientAdmin = "admin"

// ErrNoAdmin means a task for the administrator found none; it retries
// until one is named.
var ErrNoAdmin = errors.New("no administrator to notify")

// contacts is Identity's contact read.
type contacts interface {
	ReadContact(ctx context.Context, userUUID string) (identity.Contact, error)
	AdminUserUUID(ctx context.Context) (string, bool, error)
}

// acceptances is Compliance's standing acceptance check.
type acceptances interface {
	HasCurrentAcceptance(ctx context.Context, userUUID string, kind compliance.TextKind) (bool, error)
}

// entryAcceptor is Notifications' acceptance.
type entryAcceptor interface {
	AcceptEntry(ctx context.Context, notice notifications.Notice) (notifications.Acceptance, error)
}

// Payload is a notify task as a domain enqueues it. Recipient is a user
// UUID or RecipientAdmin; IdempotencyKey is unique within the domain;
// Student marks a recipient acting as a student; SendAt is optional.
type Payload struct {
	Recipient      string         `json:"recipient"`
	Type           string         `json:"type"`
	IdempotencyKey string         `json:"idempotency_key"`
	ExternalToken  string         `json:"external_token"`
	Skeleton       map[string]any `json:"skeleton"`
	SendAt         time.Time      `json:"send_at"`
	Student        bool           `json:"student"`
}

// Delivery is the notify delivery.
type Delivery struct {
	contacts    contacts
	acceptances acceptances
	entries     entryAcceptor
}

// New constructs a Delivery.
func New(contacts contacts, acceptances acceptances, entries entryAcceptor) *Delivery {
	return &Delivery{contacts: contacts, acceptances: acceptances, entries: entries}
}

// Register handles notify tasks on each queue; the queue's domain is the
// notice's source.
func (d *Delivery) Register(queues ...*txqueue.Queue) {
	for _, queue := range queues {
		source := queue.Domain()
		queue.HandleDelivery(txqueue.TypeNotify, func(ctx context.Context, task txqueue.Task) error {
			return d.Deliver(ctx, source, task.Payload)
		})
	}
}

// Deliver turns one notify payload from source into an entry. A payload
// Notifications cannot read, or an unknown recipient, turns the task dead.
func (d *Delivery) Deliver(ctx context.Context, source string, raw json.RawMessage) error {
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return txqueue.Permanent(fmt.Errorf("reading notify task from %s: %w", source, err))
	}
	recipient := p.Recipient
	if recipient == RecipientAdmin {
		admin, found, err := d.contacts.AdminUserUUID(ctx)
		if err != nil {
			return err
		}
		if !found {
			return ErrNoAdmin
		}
		recipient = admin
	}
	contact, err := d.contacts.ReadContact(ctx, recipient)
	if err != nil {
		return err
	}
	if !contact.Found {
		return txqueue.Permanent(fmt.Errorf("notify task from %s with key %q names an unknown recipient", source, p.IdempotencyKey))
	}
	consent := false
	if notifications.Type(p.Type) == notifications.TypeMarketing {
		if consent, err = d.acceptances.HasCurrentAcceptance(ctx, recipient, compliance.TextMarketing); err != nil {
			return err
		}
	}
	result, err := d.entries.AcceptEntry(ctx, notifications.Notice{
		SourceDomain: source, Recipient: recipient, Type: notifications.Type(p.Type), IdempotencyKey: p.IdempotencyKey,
		ExternalToken: p.ExternalToken, Skeleton: p.Skeleton, SendAt: p.SendAt,
		HasEmail: contact.Email != "", Student: p.Student, MarketingConsent: consent,
	})
	if err != nil {
		return err
	}
	if result.Outcome == notifications.AcceptInvalid {
		return txqueue.Permanent(fmt.Errorf("invalid notify task from %s with key %q", source, p.IdempotencyKey))
	}
	return nil
}
