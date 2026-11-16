package accountlogin

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/login"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/accountlogin/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

var (
	linkOutcomeLabel = metrics.NewLabel("outcome", string(identity.MethodLinked), string(identity.MethodInUse),
		string(identity.MethodInvalidCode), string(identity.MethodAttemptGone))

	providerLinksTotal = metrics.NewCounter(
		"identity_provider_method_links_total",
		"Attempts to add a provider identity as a way into an account, by what was decided.",
		linkOutcomeLabel,
	)
)

// StartLinkProvider starts the round trip that adds provider to userUUID's
// account: the same machinery as a sign-in, with the attempt bound to the
// calling account, so coming back links instead of opening a session.
func (m *Manager) StartLinkProvider(ctx context.Context, userUUID string, provider string, codeChallenge string) (identity.StartLoginResult, error) {
	defer logging.Step(ctx, "accountlogin.start_link_provider").Close()
	return m.start(ctx, provider, codeChallenge, internalrepo.IntentLink, "", userUUID)
}

// CompleteLinkProvider takes the one-time code the callback issued for a
// link attempt of userUUID and answers what the callback decided: the
// identity is now another way into that account, or it belongs to another
// one, which never merges them.
func (m *Manager) CompleteLinkProvider(ctx context.Context, userUUID string, code string, codeVerifier string) (identity.LinkMethodResult, error) {
	defer logging.Step(ctx, "accountlogin.complete_link_provider").Close()

	now := m.now().UTC().Truncate(time.Microsecond)
	outcome, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (identity.LinkMethodOutcome, error) {
		return m.completeLinkInTx(ctx, tx, userUUID, code, codeVerifier, now)
	})
	if err != nil {
		return identity.LinkMethodResult{}, err
	}
	providerLinksTotal.Inc(ctx, linkOutcomeLabel.Value(string(outcome)))
	return identity.LinkMethodResult{Outcome: outcome}, nil
}

func (m *Manager) completeLinkInTx(ctx context.Context, tx *gorm.DB, userUUID string, code string, codeVerifier string, now time.Time) (identity.LinkMethodOutcome, error) {
	if code == "" {
		return identity.MethodAttemptGone, nil
	}
	attempt, err := m.completeRepo.LockAttemptByCode(ctx, tx, code)
	if err != nil || attempt == nil {
		return identity.MethodAttemptGone, err
	}
	rejected := func(reason string, outcome identity.LinkMethodOutcome) (identity.LinkMethodOutcome, error) {
		return outcome, m.completeRepo.CloseAttempt(ctx, tx, attempt.ID, internalrepo.AttemptEvent{Event: internalrepo.EventRejected, Reason: reason, OccurredAt: now})
	}
	switch {
	case internalrepo.Intent(attempt.Intent) != internalrepo.IntentLink || attempt.UserUUID != userUUID:
		return identity.MethodAttemptGone, nil
	case attempt.CompletedAt != nil:
		return rejected("used", identity.MethodAttemptGone)
	case !now.Before(attempt.ExpiresAt):
		return rejected("expired", identity.MethodAttemptGone)
	case !login.VerifierMatches(attempt.CodeChallenge, codeVerifier):
		return rejected("verifier_mismatch", identity.MethodInvalidCode)
	}
	outcome := identity.MethodLinked
	if attempt.IssuedReason == internalrepo.ReasonMethodInUse {
		outcome = identity.MethodInUse
	}
	err = m.completeRepo.CloseAttempt(ctx, tx, attempt.ID, internalrepo.AttemptEvent{
		Event: internalrepo.EventCompleted, Reason: attempt.IssuedReason, UserID: attempt.UserID, OccurredAt: now,
	})
	return outcome, err
}
