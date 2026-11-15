// Package secondfactor is the administrator's second factor: enrolling a
// TOTP credential, verifying codes for an admin-mode token or a single-use
// step-up proof, and spending such a proof on one sensitive action.
package secondfactor

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/accesstoken"
	"github.com/diegobermudez03/playhoot/identity/internal/business/totp"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/secondfactor/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

// What a check row records: why the code was asked for, and how it ended.
const (
	purposeReEnroll = "re_enroll"
	purposeConfirm  = "confirm_enrollment"

	checkAccepted    = "accepted"
	checkWrongCode   = "wrong_code"
	checkReplayed    = "replayed"
	checkLocked      = "locked"
	checkNotAdmin    = "not_admin"
	checkNotEnrolled = "not_enrolled"
)

// failureOutcomes are the checks that count toward the lock.
var failureOutcomes = []string{checkWrongCode, checkReplayed}

var (
	operationLabel = metrics.NewLabel("operation", "enroll", "confirm", "verify_admin_token", "verify_step_up", "consume_step_up")
	outcomeLabel   = metrics.NewLabel("outcome",
		string(identity.SecondFactorAccepted), string(identity.SecondFactorNotAdmin), string(identity.SecondFactorNotEnrolled),
		string(identity.SecondFactorNoPendingEnrollment), string(identity.SecondFactorCurrentCodeRequired),
		string(identity.SecondFactorInvalidCode), string(identity.SecondFactorLocked),
		string(identity.StepUpConsumed), string(identity.StepUpUnknown), string(identity.StepUpExpired), string(identity.StepUpUsed))

	secondFactorTotal = metrics.NewCounter(
		"identity_second_factor_total",
		"Second-factor operations by what they decided: enrollments, confirmations, verifications and step-up proofs spent.",
		operationLabel, outcomeLabel,
	)
)

// Service is the second factor use case.
type Service struct {
	repo      *internalrepo.Repo
	sealer    *totp.Sealer
	signer    *accesstoken.Signer
	now       func() time.Time
	newSecret func() (string, error)
	newProof  func() (string, error)
}

// New constructs a Service on db, sealing secrets with totpKey (32 bytes)
// and signing admin-mode tokens with accessKey.
func New(db *gorm.DB, totpKey []byte, accessKey []byte) (*Service, error) {
	sealer, err := totp.NewSealer(totpKey)
	if err != nil {
		return nil, err
	}
	signer, err := accesstoken.NewSigner(accessKey)
	if err != nil {
		return nil, err
	}
	return &Service{repo: internalrepo.New(db), sealer: sealer, signer: signer, now: time.Now, newSecret: totp.NewSecret, newProof: totp.NewProof}, nil
}

// EnrollTotp starts an enrollment for the administrator userUUID. Replacing
// a confirmed credential needs currentCode from it; the new one is unusable
// until ConfirmTotp. identity.ErrUserNotFound when there is no such User.
func (s *Service) EnrollTotp(ctx context.Context, userUUID string, currentCode string) (identity.TotpEnrollment, error) {
	defer logging.Step(ctx, "secondfactor.enroll").Close()
	now := s.clock()
	enrollment, err := utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (identity.TotpEnrollment, error) {
		user, err := s.lockAdmin(ctx, tx, userUUID)
		if err != nil || user == nil {
			return identity.TotpEnrollment{Outcome: identity.SecondFactorNotAdmin}, err
		}
		confirmed, err := s.repo.ConfirmedCredential(ctx, tx, user.ID)
		if err != nil {
			return identity.TotpEnrollment{}, err
		}
		if confirmed != nil {
			if currentCode == "" {
				return identity.TotpEnrollment{Outcome: identity.SecondFactorCurrentCodeRequired}, nil
			}
			checked, err := s.checkCode(ctx, tx, user.ID, confirmed, currentCode, purposeReEnroll, now)
			if err != nil || checked.outcome != identity.SecondFactorAccepted {
				return identity.TotpEnrollment{Outcome: checked.outcome, LockedUntil: checked.lockedUntil}, err
			}
			if err := s.recordAccepted(ctx, tx, user.ID, purposeReEnroll, checked.step, nil, now); err != nil {
				return identity.TotpEnrollment{}, err
			}
		}
		secret, err := s.newSecret()
		if err != nil {
			return identity.TotpEnrollment{}, err
		}
		sealed, err := s.sealer.Seal(secret)
		if err != nil {
			return identity.TotpEnrollment{}, err
		}
		if err := s.repo.RevokePending(ctx, tx, user.ID, now); err != nil {
			return identity.TotpEnrollment{}, err
		}
		if err := s.repo.CreatePendingCredential(ctx, tx, user.ID, sealed, now); err != nil {
			return identity.TotpEnrollment{}, err
		}
		return identity.TotpEnrollment{Outcome: identity.SecondFactorAccepted, Secret: secret, URI: totp.URI(secret, userUUID), ExpiresAt: now.Add(totp.PendingLifetime)}, nil
	})
	if err != nil {
		return identity.TotpEnrollment{}, err
	}
	s.count(ctx, "enroll", enrollment.Outcome)
	return enrollment, nil
}

// ConfirmTotp confirms the administrator's pending enrollment with its first
// valid code, replacing any confirmed credential.
func (s *Service) ConfirmTotp(ctx context.Context, userUUID string, code string) (identity.TotpConfirmation, error) {
	defer logging.Step(ctx, "secondfactor.confirm").Close()
	now := s.clock()
	confirmation, err := utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (identity.TotpConfirmation, error) {
		user, err := s.lockAdmin(ctx, tx, userUUID)
		if err != nil || user == nil {
			return identity.TotpConfirmation{Outcome: identity.SecondFactorNotAdmin}, err
		}
		pending, err := s.repo.PendingCredential(ctx, tx, user.ID)
		if err != nil {
			return identity.TotpConfirmation{}, err
		}
		if pending == nil || !now.Before(pending.CreatedAt.Add(totp.PendingLifetime)) {
			return identity.TotpConfirmation{Outcome: identity.SecondFactorNoPendingEnrollment}, nil
		}
		checked, err := s.checkCode(ctx, tx, user.ID, pending, code, purposeConfirm, now)
		if err != nil || checked.outcome != identity.SecondFactorAccepted {
			return identity.TotpConfirmation{Outcome: checked.outcome, LockedUntil: checked.lockedUntil}, err
		}
		if err := s.recordAccepted(ctx, tx, user.ID, purposeConfirm, checked.step, nil, now); err != nil {
			return identity.TotpConfirmation{}, err
		}
		if err := s.repo.RevokeConfirmed(ctx, tx, user.ID, now); err != nil {
			return identity.TotpConfirmation{}, err
		}
		if err := s.repo.Confirm(ctx, tx, pending.ID, now); err != nil {
			return identity.TotpConfirmation{}, err
		}
		return identity.TotpConfirmation{Outcome: identity.SecondFactorAccepted}, nil
	})
	if err != nil {
		return identity.TotpConfirmation{}, err
	}
	s.count(ctx, "confirm", confirmation.Outcome)
	return confirmation, nil
}

// VerifyTotp checks the administrator's code and exchanges it for purpose:
// an admin-mode access token, or a step-up proof spendable once for 10
// minutes. Every attempt is recorded.
func (s *Service) VerifyTotp(ctx context.Context, userUUID string, code string, purpose identity.SecondFactorPurpose) (identity.TotpVerification, error) {
	defer logging.Step(ctx, "secondfactor.verify").Close()
	if purpose != identity.PurposeAdminToken && purpose != identity.PurposeStepUp {
		return identity.TotpVerification{}, fmt.Errorf("unknown second factor purpose %q", purpose)
	}
	now := s.clock()
	var displayName string
	var proof string
	verification, err := utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (identity.TotpVerification, error) {
		user, err := s.repo.LockUser(ctx, tx, userUUID)
		if err != nil {
			return identity.TotpVerification{}, err
		}
		if user == nil {
			return identity.TotpVerification{}, identity.ErrUserNotFound
		}
		if user.DisplayName != nil {
			displayName = *user.DisplayName
		}
		admin, err := s.repo.HasActiveGrant(ctx, tx, user.ID)
		if err != nil {
			return identity.TotpVerification{}, err
		}
		if !admin {
			return identity.TotpVerification{Outcome: identity.SecondFactorNotAdmin}, s.record(ctx, tx, user.ID, string(purpose), checkNotAdmin, now)
		}
		credential, err := s.repo.ConfirmedCredential(ctx, tx, user.ID)
		if err != nil {
			return identity.TotpVerification{}, err
		}
		if credential == nil {
			return identity.TotpVerification{Outcome: identity.SecondFactorNotEnrolled}, s.record(ctx, tx, user.ID, string(purpose), checkNotEnrolled, now)
		}
		checked, err := s.checkCode(ctx, tx, user.ID, credential, code, string(purpose), now)
		if err != nil || checked.outcome != identity.SecondFactorAccepted {
			return identity.TotpVerification{Outcome: checked.outcome, LockedUntil: checked.lockedUntil}, err
		}
		verification := identity.TotpVerification{Outcome: identity.SecondFactorAccepted}
		var accepted *internalrepo.Check
		if purpose == identity.PurposeStepUp {
			proof, err = s.newProof()
			if err != nil {
				return identity.TotpVerification{}, err
			}
			hash := totp.HashProof(proof)
			expiresAt := now.Add(totp.ProofLifetime)
			accepted = &internalrepo.Check{ProofSHA256: &hash, ExpiresAt: &expiresAt}
			verification.StepUpProof, verification.StepUpExpiry = proof, expiresAt
		}
		return verification, s.recordAccepted(ctx, tx, user.ID, string(purpose), checked.step, accepted, now)
	})
	if err != nil {
		return identity.TotpVerification{}, err
	}
	if verification.Outcome == identity.SecondFactorAccepted && purpose == identity.PurposeAdminToken {
		admin := identity.User{UUID: userUUID, DisplayName: displayName, Level: identity.LevelAdmin}
		token, expiresAt, err := s.signer.Issue(admin, now)
		if err != nil {
			return identity.TotpVerification{}, err
		}
		verification.Credentials = identity.Credentials{User: admin, AccessToken: token, AccessIssuedAt: now, AccessExpiresAt: expiresAt}
	}
	s.count(ctx, "verify_"+string(purpose), verification.Outcome)
	return verification, nil
}

// ConsumeStepUpProof spends userUUID's step-up proof on action (what the
// sensitive operation is called), at most once and before it expires.
func (s *Service) ConsumeStepUpProof(ctx context.Context, userUUID string, proof string, action string) (identity.StepUpOutcome, error) {
	if !totp.ValidAction(action) {
		return "", fmt.Errorf("invalid step-up action %q", action)
	}
	outcome := identity.StepUpUnknown
	if totp.WellFormedProof(proof) {
		hash := totp.HashProof(proof)
		now := s.clock()
		consumed, err := s.repo.ConsumeProof(ctx, userUUID, string(identity.PurposeStepUp), hash, action, now)
		if err != nil {
			return "", err
		}
		if consumed {
			outcome = identity.StepUpConsumed
		} else {
			found, err := s.repo.FindProof(ctx, userUUID, string(identity.PurposeStepUp), hash)
			if err != nil {
				return "", err
			}
			switch {
			case found == nil:
			case found.UsedAt != nil:
				outcome = identity.StepUpUsed
			default:
				outcome = identity.StepUpExpired
			}
		}
	}
	secondFactorTotal.Inc(ctx, operationLabel.Value("consume_step_up"), outcomeLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("step_up_action", action), logging.Field("step_up_outcome", string(outcome)))
	return outcome, nil
}

func (s *Service) clock() time.Time {
	return s.now().UTC().Truncate(time.Microsecond)
}

// lockAdmin locks userUUID's User and returns it only if they hold the grant.
func (s *Service) lockAdmin(ctx context.Context, tx *gorm.DB, userUUID string) (*internalrepo.User, error) {
	user, err := s.repo.LockUser(ctx, tx, userUUID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, identity.ErrUserNotFound
	}
	admin, err := s.repo.HasActiveGrant(ctx, tx, user.ID)
	if err != nil || !admin {
		return nil, err
	}
	return user, nil
}

type checkedCode struct {
	outcome     identity.SecondFactorOutcome
	lockedUntil time.Time
	step        int64
}

// checkCode evaluates code against credential, recording every refusal. An
// accepted code is left for the caller to record with what it bought.
func (s *Service) checkCode(ctx context.Context, tx *gorm.DB, userID int64, credential *internalrepo.Credential, code string, purpose string, now time.Time) (checkedCode, error) {
	failures, err := s.repo.RecentFailures(ctx, tx, userID, failureOutcomes, now.Add(-(totp.FailureWindow + totp.LockDuration)), totp.MaxFailures)
	if err != nil {
		return checkedCode{}, err
	}
	if until, locked := totp.LockedUntil(failures, now); locked {
		return checkedCode{outcome: identity.SecondFactorLocked, lockedUntil: until}, s.record(ctx, tx, userID, purpose, checkLocked, now)
	}
	secret, err := s.sealer.Open(credential.SecretEncrypted)
	if err != nil {
		return checkedCode{}, err
	}
	step, matched, err := totp.Match(secret, code, now)
	if err != nil {
		return checkedCode{}, err
	}
	refusal := ""
	if !matched {
		refusal = checkWrongCode
	} else {
		last, has, err := s.repo.LastAcceptedStep(ctx, tx, userID, checkAccepted)
		if err != nil {
			return checkedCode{}, err
		}
		if has && step <= last {
			refusal = checkReplayed
		}
	}
	if refusal == "" {
		return checkedCode{outcome: identity.SecondFactorAccepted, step: step}, nil
	}
	if err := s.record(ctx, tx, userID, purpose, refusal, now); err != nil {
		return checkedCode{}, err
	}
	failures = append([]time.Time{now}, failures...)
	if len(failures) > totp.MaxFailures {
		failures = failures[:totp.MaxFailures]
	}
	if until, locked := totp.LockedUntil(failures, now); locked {
		return checkedCode{outcome: identity.SecondFactorLocked, lockedUntil: until}, nil
	}
	return checkedCode{outcome: identity.SecondFactorInvalidCode}, nil
}

func (s *Service) record(ctx context.Context, tx *gorm.DB, userID int64, purpose string, outcome string, now time.Time) error {
	return s.repo.RecordCheck(ctx, tx, internalrepo.Check{UserID: userID, Purpose: purpose, Outcome: outcome, CreatedAt: now})
}

func (s *Service) recordAccepted(ctx context.Context, tx *gorm.DB, userID int64, purpose string, step int64, check *internalrepo.Check, now time.Time) error {
	row := internalrepo.Check{}
	if check != nil {
		row = *check
	}
	row.UserID, row.Purpose, row.Outcome, row.TimeStep, row.CreatedAt = userID, purpose, checkAccepted, &step, now
	return s.repo.RecordCheck(ctx, tx, row)
}

func (s *Service) count(ctx context.Context, operation string, outcome identity.SecondFactorOutcome) {
	secondFactorTotal.Inc(ctx, operationLabel.Value(operation), outcomeLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("second_factor_"+operation, string(outcome)))
}
