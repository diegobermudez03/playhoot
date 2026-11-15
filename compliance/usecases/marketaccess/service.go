// Package marketaccess answers whether a function is available to a person
// in a place, and records their residence-and-age declaration.
package marketaccess

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/compliance"
	rules "github.com/diegobermudez03/playhoot/compliance/internal/business/marketaccess"
	internalrepo "github.com/diegobermudez03/playhoot/compliance/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

// Who recorded a minority signal: a declaration below the age asked for.
const minorityFromDeclaration = "declaration"

// lockDeclaration serializes one person's declarations.
const lockDeclaration = "compliance-declaration"

var (
	functionLabel = metrics.NewLabel("function", functionNames()...)
	accessLabel   = metrics.NewLabel("outcome",
		string(compliance.AccessAllowed), string(compliance.AccessNeedsDeclaration),
		string(compliance.BlockMarketUnavailable), string(compliance.BlockAgeRequirement))
	accessTotal = metrics.NewCounter(
		"compliance_market_access_total",
		"Market access checks by function and decision (blocked ones by reason).",
		functionLabel, accessLabel,
	)

	originLabel      = metrics.NewLabel("origin", string(compliance.DeclaredByUser), string(compliance.DeclaredBySupport))
	declarationLabel = metrics.NewLabel("outcome",
		string(compliance.DeclarationRecorded), string(compliance.DeclarationUnchanged),
		string(compliance.DeclarationConflict), string(compliance.DeclarationInvalid))
	declarationTotal = metrics.NewCounter(
		"compliance_declarations_total",
		"Residence-and-age declarations by origin and what was decided.",
		originLabel, declarationLabel,
	)
)

func functionNames() []string {
	names := make([]string, len(compliance.Functions))
	for i, function := range compliance.Functions {
		names[i] = string(function)
	}
	return names
}

// Service is the market access use case.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// CheckMarketAccess answers whether request.Function is available to the
// person in their declared residence or, without one, approximate location.
func (s *Service) CheckMarketAccess(ctx context.Context, request compliance.AccessRequest) (compliance.AccessDecision, error) {
	defer logging.Step(ctx, "compliance.check_market_access").Close()
	if !rules.KnownFunction(request.Function) {
		return compliance.AccessDecision{}, fmt.Errorf("unknown function %q", request.Function)
	}
	now := s.now()
	db := s.repo.GetDB()
	facts := rules.Facts{Function: request.Function}
	if rules.ValidCountry(request.ApproximateCountry) {
		facts.ApproximateCountry = request.ApproximateCountry
	}
	if request.UserUUID != "" {
		if _, err := uuid.Parse(request.UserUUID); err != nil {
			return compliance.AccessDecision{}, fmt.Errorf("invalid user uuid: %w", err)
		}
		declaration, err := s.repo.LatestDeclaration(ctx, db, request.UserUUID)
		if err != nil {
			return compliance.AccessDecision{}, err
		}
		if declaration != nil {
			facts.Declaration = &rules.Declared{Country: declaration.Country, Subdivision: declaration.Subdivision, MeetsAge: declaration.MeetsAge, ThresholdAge: declaration.ThresholdAge}
		}
		if facts.Minority, err = s.repo.HasMinoritySignal(ctx, db, request.UserUUID); err != nil {
			return compliance.AccessDecision{}, err
		}
	}
	if country, subdivision, ok := rules.Place(facts.Declaration, facts.ApproximateCountry); ok {
		rule, err := s.repo.CurrentRule(ctx, db, country, subdivision, string(request.Function), now)
		if err != nil {
			return compliance.AccessDecision{}, err
		}
		if rule != nil {
			facts.Rule = &rules.Rule{Enabled: rule.Enabled, MinAge: rule.MinAge}
		} else if facts.CountryCovered, _, err = s.repo.CountryCoverage(ctx, db, country, now); err != nil {
			return compliance.AccessDecision{}, err
		}
	}
	decision := rules.Decide(facts)
	outcome := string(decision.Outcome)
	if decision.Outcome == compliance.AccessBlocked {
		outcome = string(decision.Reason)
	}
	accessTotal.Inc(ctx, functionLabel.Value(string(request.Function)), accessLabel.Value(outcome))
	logging.LogFields(ctx, logging.Field("market_access", outcome))
	return decision, nil
}

// RecordDeclaration records a residence-and-age declaration. A person's
// different answer is refused; support's correction is always recorded. A
// declaration below the age asked for records a binding minority signal.
func (s *Service) RecordDeclaration(ctx context.Context, declaration compliance.Declaration) (compliance.DeclarationResult, error) {
	defer logging.Step(ctx, "compliance.record_declaration").Close()
	if declaration.Origin != compliance.DeclaredByUser && declaration.Origin != compliance.DeclaredBySupport {
		return compliance.DeclarationResult{}, fmt.Errorf("unknown declaration origin %q", declaration.Origin)
	}
	if _, err := uuid.Parse(declaration.UserUUID); err != nil {
		return compliance.DeclarationResult{}, fmt.Errorf("invalid user uuid: %w", err)
	}
	now := s.now()
	result, err := utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (compliance.DeclarationResult, error) {
		if !rules.ValidCountry(declaration.Country) || !rules.ValidSubdivision(declaration.Subdivision) {
			return compliance.DeclarationResult{Outcome: compliance.DeclarationInvalid}, nil
		}
		_, perSubdivision, err := s.repo.CountryCoverage(ctx, tx, declaration.Country, now)
		if err != nil {
			return compliance.DeclarationResult{}, err
		}
		subdivision := declaration.Subdivision
		if !perSubdivision {
			subdivision = ""
		} else if subdivision == "" {
			return compliance.DeclarationResult{Outcome: compliance.DeclarationInvalid}, nil
		}
		threshold, err := s.threshold(ctx, tx, declaration.Country, subdivision, now)
		if err != nil {
			return compliance.DeclarationResult{}, err
		}
		next := rules.Declared{Country: declaration.Country, Subdivision: subdivision, MeetsAge: declaration.MeetsAge, ThresholdAge: threshold}
		if err := s.repo.LockPerson(ctx, tx, lockDeclaration, declaration.UserUUID); err != nil {
			return compliance.DeclarationResult{}, err
		}
		previous, err := s.repo.LatestDeclaration(ctx, tx, declaration.UserUUID)
		if err != nil {
			return compliance.DeclarationResult{}, err
		}
		if previous != nil {
			outcome := rules.Compare(rules.Declared{Country: previous.Country, Subdivision: previous.Subdivision, MeetsAge: previous.MeetsAge, ThresholdAge: previous.ThresholdAge}, next)
			if outcome == compliance.DeclarationUnchanged {
				return compliance.DeclarationResult{Outcome: outcome, ThresholdAge: previous.ThresholdAge}, nil
			}
			if outcome == compliance.DeclarationConflict && declaration.Origin == compliance.DeclaredByUser {
				return compliance.DeclarationResult{Outcome: outcome, ThresholdAge: previous.ThresholdAge}, nil
			}
		}
		err = s.repo.InsertDeclaration(ctx, tx, internalrepo.Declaration{
			UserUUID: declaration.UserUUID, Country: next.Country, Subdivision: next.Subdivision,
			MeetsAge: next.MeetsAge, ThresholdAge: threshold, Origin: string(declaration.Origin), CreatedAt: now,
		})
		if err != nil {
			return compliance.DeclarationResult{}, err
		}
		if !next.MeetsAge {
			err := s.repo.InsertMinoritySignal(ctx, tx, internalrepo.MinoritySignal{UserUUID: declaration.UserUUID, Origin: minorityFromDeclaration, CreatedAt: now})
			if err != nil {
				return compliance.DeclarationResult{}, err
			}
		}
		return compliance.DeclarationResult{Outcome: compliance.DeclarationRecorded, ThresholdAge: threshold}, nil
	})
	if err != nil {
		return compliance.DeclarationResult{}, err
	}
	declarationTotal.Inc(ctx, originLabel.Value(string(declaration.Origin)), declarationLabel.Value(string(result.Outcome)))
	logging.LogFields(ctx, logging.Field("declaration", string(result.Outcome)))
	return result, nil
}

// threshold is the highest age the place's rules in force ask for, or the
// default where the matrix does not cover it.
func (s *Service) threshold(ctx context.Context, tx *gorm.DB, country, subdivision string, now time.Time) (int, error) {
	placeRules, err := s.repo.PlaceRules(ctx, tx, country, subdivision, now)
	if err != nil {
		return 0, err
	}
	threshold := 0
	for _, rule := range placeRules {
		if rule.MinAge > threshold {
			threshold = rule.MinAge
		}
	}
	if threshold == 0 {
		threshold = rules.DefaultThreshold
	}
	return threshold, nil
}
