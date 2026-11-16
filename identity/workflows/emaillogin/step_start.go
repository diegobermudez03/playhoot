package emaillogin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/emailcode"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/emaillogin/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

//go:generate mockgen -package=emaillogin -destination=step_start_mock_test.go . startRepo,Enqueuer,accessIssuer

// startRepo is StartEmailLogin's own narrow persistence contract.
type startRepo interface {
	SendsSince(ctx context.Context, tx *gorm.DB, email string, burstSince time.Time, daySince time.Time) (int, int, error)
	SupersedeLive(ctx context.Context, tx *gorm.DB, email string, at time.Time) error
	CreateAttempt(ctx context.Context, tx *gorm.DB, attempt internalrepo.NewAttempt) error
}

// notificationType is the kind of notification an access code is, as
// Notifications names it.
const notificationType = "access-code"

// notification is the notify task payload: the code's email, addressed to
// the attempt itself, so no address is stored outside Identity.
type notification struct {
	Recipient      string         `json:"recipient"`
	Type           string         `json:"type"`
	IdempotencyKey string         `json:"idempotency_key"`
	ExternalToken  string         `json:"external_token"`
	Skeleton       map[string]any `json:"skeleton"`
}

var (
	startOutcomeLabel = metrics.NewLabel("outcome", "sent", "rate_limited", "invalid_address")
	startIntentLabel  = metrics.NewLabel("intent", string(internalrepo.IntentSignIn), string(internalrepo.IntentLink))

	startsTotal = metrics.NewCounter(
		"identity_email_code_starts_total",
		"One-time email codes asked for, by what was decided and what the code is for.",
		startOutcomeLabel, startIntentLabel,
	)
)

// StartEmailLogin sends a one-time code to address, in the language asked
// in, so the person signs in or finishes registering. guestUserUUID, when
// set, is the guest the client is: an account the code creates is theirs.
// The answer never says whether a code was sent.
func (m *Manager) StartEmailLogin(ctx context.Context, address string, language string, guestUserUUID string) (identity.StartEmailLoginResult, error) {
	defer logging.Step(ctx, "emaillogin.start_email_login").Close()
	return m.start(ctx, address, language, internalrepo.IntentSignIn, guestUserUUID, "")
}

// start records an attempt and leaves its email in the queue, unless the
// address has been sent too many codes lately. Its answer is the same
// either way.
func (m *Manager) start(ctx context.Context, address string, language string, intent internalrepo.Intent, guestUserUUID string, linkUserUUID string) (identity.StartEmailLoginResult, error) {
	normalized, ok := emailcode.Normalize(address)
	if !ok {
		startsTotal.Inc(ctx, startOutcomeLabel.Value("invalid_address"), startIntentLabel.Value(string(intent)))
		return identity.StartEmailLoginResult{Outcome: identity.EmailLoginInvalidAddress}, nil
	}
	code, err := emailcode.NewCode()
	if err != nil {
		return identity.StartEmailLoginResult{}, err
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	sent, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		return m.startInTx(ctx, tx, normalized, language, intent, guestUserUUID, linkUserUUID, code, now)
	})
	if err != nil {
		return identity.StartEmailLoginResult{}, err
	}
	outcome := "sent"
	if !sent {
		outcome = "rate_limited"
	}
	startsTotal.Inc(ctx, startOutcomeLabel.Value(outcome), startIntentLabel.Value(string(intent)))
	return identity.StartEmailLoginResult{Outcome: identity.EmailLoginAcknowledged}, nil
}

func (m *Manager) startInTx(ctx context.Context, tx *gorm.DB, address string, language string, intent internalrepo.Intent, guestUserUUID string, linkUserUUID string, code string, now time.Time) (bool, error) {
	burst, day, err := m.startRepo.SendsSince(ctx, tx, address, now.Add(-emailcode.BurstWindow), now.Add(-emailcode.DayWindow))
	if err != nil {
		return false, err
	}
	if burst >= emailcode.BurstSends || day >= emailcode.DaySends {
		return false, nil
	}
	if err := m.startRepo.SupersedeLive(ctx, tx, address, now); err != nil {
		return false, err
	}
	attemptUUID := uuid.NewString()
	attempt := internalrepo.NewAttempt{
		AttemptUUID:   attemptUUID,
		Email:         address,
		Intent:        intent,
		CodeHash:      emailcode.Hash(code),
		Language:      language,
		GuestUserUUID: guestUserUUID,
		LinkUserUUID:  linkUserUUID,
		ExpiresAt:     now.Add(emailcode.Lifetime),
		CreatedAt:     now,
	}
	if err := m.startRepo.CreateAttempt(ctx, tx, attempt); err != nil {
		return false, err
	}
	// The attempt is the recipient: Identity resolves it to the address, so
	// Notifications never stores one and a code reaches a person who has no
	// User yet.
	task := notification{
		Recipient: attemptUUID, Type: notificationType, IdempotencyKey: attemptUUID, ExternalToken: attemptUUID,
		Skeleton: map[string]any{"code": code},
	}
	if err := m.queue.Enqueue(tx, txqueue.TypeNotify, task, time.Time{}, attemptUUID); err != nil {
		return false, err
	}
	return true, nil
}
