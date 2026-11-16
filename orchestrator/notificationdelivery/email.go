package notificationdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// emailWorkWindow bounds one email send as admitted work.
const emailWorkWindow = 2 * time.Minute

// ErrEmailPaused means the email waits: the budget refused it, or no
// provider took it yet. The task retries; the in-app entry already exists.
var ErrEmailPaused = errors.New("email paused")

// admitter is Budget's admission.
type admitter interface {
	AdmitWork(ctx context.Context, request budget.WorkRequest) (budget.Admission, error)
}

// settler is Budget's settlement.
type settler interface {
	SettleWork(ctx context.Context, reservationUUID string, settlement budget.Settlement) (budget.SettleOutcome, error)
}

// emailSender is Notifications' email send.
type emailSender interface {
	SendEmail(ctx context.Context, entryUUID string, to notifications.Recipient) (notifications.SendOutcome, error)
}

// recipientReader is Identity's contact read.
type recipientReader interface {
	ReadContact(ctx context.Context, userUUID string) (identity.Contact, error)
}

// Email runs email tasks.
type Email struct {
	contacts recipientReader
	admit    admitter
	send     emailSender
	settle   settler
}

// NewEmail constructs an Email.
func NewEmail(contacts recipientReader, admit admitter, send emailSender, settle settler) *Email {
	return &Email{contacts: contacts, admit: admit, send: send, settle: settle}
}

// Register handles Notifications' email tasks.
func (e *Email) Register(queue *txqueue.Queue) {
	queue.Handle(notifications.SendTaskType, func(ctx context.Context, task txqueue.Task) error {
		return e.Run(ctx, task.Payload)
	})
}

// Run reads the recipient's current address, admits the email as
// cost-bearing work, sends it and releases the reservation: the measured
// spend arrives through Notifications' own spend events.
func (e *Email) Run(ctx context.Context, raw json.RawMessage) error {
	var task notifications.SendTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return txqueue.Permanent(fmt.Errorf("reading email task: %w", err))
	}
	contact, err := e.contacts.ReadContact(ctx, task.Recipient)
	if err != nil {
		return err
	}
	language := notifications.Language(contact.Language)
	if language == "" {
		language = notifications.DefaultLanguage
	}
	admission, err := e.admit.AdmitWork(ctx, budget.WorkRequest{
		Category: budget.Category(notifications.SpendCategory), Service: notifications.SpendService(notifications.ProviderSES),
		Quantity: 1, Reference: notifications.EmailReference(task.EntryUUID), Discretionary: !task.Necessary, MaxDuration: emailWorkWindow,
	})
	if err != nil {
		return err
	}
	if admission.Outcome != budget.Admitted {
		logging.LogFields(ctx, logging.Field("email_refused", string(admission.Reason)))
		return fmt.Errorf("%w: %s", ErrEmailPaused, admission.Reason)
	}
	outcome, sendErr := e.send.SendEmail(ctx, task.EntryUUID, notifications.Recipient{Email: contact.Email, Language: language})
	if _, err := e.settle.SettleWork(ctx, admission.ReservationUUID, budget.Settlement{}); err != nil {
		// The reservation expires on its own; the send's result stands.
		logging.LogError(ctx, err)
	}
	if sendErr != nil {
		return sendErr
	}
	if outcome == notifications.EmailRetry {
		return ErrEmailPaused
	}
	return nil
}
