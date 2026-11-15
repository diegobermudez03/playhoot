// Package acceptances serves the versioned texts people accept and records
// their acceptances and marketing withdrawals.
package acceptances

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/compliance"
	internalrepo "github.com/diegobermudez03/playhoot/compliance/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

// lockAcceptance serializes one person's acceptances.
const lockAcceptance = "compliance-acceptance"

// contextPattern is where an acceptance happened, such as registration.
var contextPattern = regexp.MustCompile(`^[a-z][a-z_-]{0,39}$`)

var (
	kindLabel    = metrics.NewLabel("kind", kindNames()...)
	actionLabel  = metrics.NewLabel("action", string(compliance.ActionAccepted), string(compliance.ActionWithdrawn))
	outcomeLabel = metrics.NewLabel("outcome",
		string(compliance.AcceptanceRecorded), string(compliance.AcceptanceUnchanged), string(compliance.AcceptanceUnknownText),
		string(compliance.AcceptanceNotWithdrawable), string(compliance.AcceptanceInvalid))
	acceptanceTotal = metrics.NewCounter(
		"compliance_acceptances_total",
		"Acceptances and withdrawals by text kind and what was decided.",
		kindLabel, actionLabel, outcomeLabel,
	)
)

func kindNames() []string {
	names := make([]string, len(compliance.TextKinds))
	for i, kind := range compliance.TextKinds {
		names[i] = string(kind)
	}
	return names
}

// Service is the acceptances use case.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// GetAcceptanceText returns the newest version of kind in language and, when
// userUUID is set, whether that person accepts this kind's newest version.
// compliance.ErrTextNotFound when there is none.
func (s *Service) GetAcceptanceText(ctx context.Context, kind compliance.TextKind, language string, userUUID string) (compliance.AcceptanceText, error) {
	defer logging.Step(ctx, "compliance.get_acceptance_text").Close()
	if !slices.Contains(compliance.TextKinds, kind) || !slices.Contains(compliance.Languages, language) {
		return compliance.AcceptanceText{}, compliance.ErrTextNotFound
	}
	text, err := s.repo.LatestText(ctx, string(kind), language)
	if err != nil {
		return compliance.AcceptanceText{}, err
	}
	if text == nil {
		return compliance.AcceptanceText{}, compliance.ErrTextNotFound
	}
	result := compliance.AcceptanceText{Kind: kind, Version: text.Version, Language: text.Language, Content: text.Content, PublishedAt: text.PublishedAt}
	if userUUID == "" {
		return result, nil
	}
	if _, err := uuid.Parse(userUUID); err != nil {
		return compliance.AcceptanceText{}, fmt.Errorf("invalid user uuid: %w", err)
	}
	result.Accepted, err = s.HasCurrentAcceptance(ctx, userUUID, kind)
	return result, err
}

// HasCurrentAcceptance reports whether the person's standing action on kind
// is accepting its newest version, in any language.
func (s *Service) HasCurrentAcceptance(ctx context.Context, userUUID string, kind compliance.TextKind) (bool, error) {
	latest, err := s.repo.LatestVersion(ctx, string(kind))
	if err != nil || latest == 0 {
		return false, err
	}
	last, err := s.repo.LastAcceptanceAction(ctx, s.repo.GetDB(), userUUID, string(kind))
	if err != nil || last == nil {
		return false, err
	}
	return last.Action == string(compliance.ActionAccepted) && last.Version == latest, nil
}

// RecordAcceptance records accepting an exact text version, or withdrawing
// the marketing consent. Repeating either changes nothing.
func (s *Service) RecordAcceptance(ctx context.Context, request compliance.AcceptanceRequest) (compliance.AcceptanceOutcome, error) {
	defer logging.Step(ctx, "compliance.record_acceptance").Close()
	outcome, err := s.recordAcceptance(ctx, request)
	if err != nil {
		return "", err
	}
	if slices.Contains(compliance.TextKinds, request.Kind) && (request.Action == compliance.ActionAccepted || request.Action == compliance.ActionWithdrawn) {
		acceptanceTotal.Inc(ctx, kindLabel.Value(string(request.Kind)), actionLabel.Value(string(request.Action)), outcomeLabel.Value(string(outcome)))
	}
	logging.LogFields(ctx, logging.Field("acceptance", string(outcome)))
	return outcome, nil
}

func (s *Service) recordAcceptance(ctx context.Context, request compliance.AcceptanceRequest) (compliance.AcceptanceOutcome, error) {
	if _, err := uuid.Parse(request.UserUUID); err != nil {
		return "", fmt.Errorf("invalid user uuid: %w", err)
	}
	if !slices.Contains(compliance.TextKinds, request.Kind) || !contextPattern.MatchString(request.Context) {
		return compliance.AcceptanceInvalid, nil
	}
	now := s.now()
	switch request.Action {
	case compliance.ActionWithdrawn:
		if request.Kind != compliance.TextMarketing {
			return compliance.AcceptanceNotWithdrawable, nil
		}
		return utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (compliance.AcceptanceOutcome, error) {
			if err := s.repo.LockPerson(ctx, tx, lockAcceptance, request.UserUUID); err != nil {
				return "", err
			}
			last, err := s.repo.LastAcceptanceAction(ctx, tx, request.UserUUID, string(request.Kind))
			if err != nil {
				return "", err
			}
			if last == nil || last.Action != string(compliance.ActionAccepted) {
				return compliance.AcceptanceUnchanged, nil
			}
			return compliance.AcceptanceRecorded, s.repo.InsertAcceptance(ctx, tx, internalrepo.Acceptance{
				UserUUID: request.UserUUID, TextID: last.TextID, Kind: string(request.Kind), Action: string(compliance.ActionWithdrawn), Context: request.Context, CreatedAt: now,
			})
		})
	case compliance.ActionAccepted:
		if request.Version < 1 || !slices.Contains(compliance.Languages, request.Language) {
			return compliance.AcceptanceInvalid, nil
		}
		text, err := s.repo.Text(ctx, string(request.Kind), request.Version, request.Language)
		if err != nil {
			return "", err
		}
		if text == nil || text.PublishedAt.After(now) {
			return compliance.AcceptanceUnknownText, nil
		}
		return utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (compliance.AcceptanceOutcome, error) {
			if err := s.repo.LockPerson(ctx, tx, lockAcceptance, request.UserUUID); err != nil {
				return "", err
			}
			last, err := s.repo.LastAcceptanceAction(ctx, tx, request.UserUUID, string(request.Kind))
			if err != nil {
				return "", err
			}
			if last != nil && last.Action == string(compliance.ActionAccepted) && last.TextID == text.ID {
				return compliance.AcceptanceUnchanged, nil
			}
			return compliance.AcceptanceRecorded, s.repo.InsertAcceptance(ctx, tx, internalrepo.Acceptance{
				UserUUID: request.UserUUID, TextID: text.ID, Kind: string(request.Kind), Action: string(compliance.ActionAccepted), Context: request.Context, CreatedAt: now,
			})
		})
	default:
		return compliance.AcceptanceInvalid, nil
	}
}
