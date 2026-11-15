// Package marketrules is admin's market matrix: reading the rules in force,
// scheduled and superseded, and appending a rule.
package marketrules

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
)

const (
	maxTextLength = 500
	maxMinAge     = 99
	// pastTolerance absorbs the clock difference between admin and server.
	pastTolerance = time.Minute
)

var (
	outcomeLabel = metrics.NewLabel("outcome",
		string(compliance.MarketRuleRecorded), string(compliance.MarketRuleConditionsPending), string(compliance.MarketRuleInvalid))
	setRuleTotal = metrics.NewCounter(
		"compliance_market_rule_changes_total",
		"Admin changes to the market matrix by what was decided.",
		outcomeLabel,
	)
)

// Service is the market matrix admin use case.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// ReadMarketRules returns the matrix, of country when not empty: the rule in
// force per place and function, scheduled rules and superseded ones.
func (s *Service) ReadMarketRules(ctx context.Context, country string) (compliance.MarketRules, error) {
	defer logging.Step(ctx, "compliance.read_market_rules").Close()
	rows, err := s.repo.MarketRules(ctx, country)
	if err != nil {
		return compliance.MarketRules{}, err
	}
	now := s.now()
	matrix := compliance.MarketRules{Current: []compliance.MarketRule{}, Scheduled: []compliance.MarketRule{}, History: []compliance.MarketRule{}}
	inForce := make(map[string]bool)
	for _, row := range rows {
		rule := toRule(row)
		key := row.Country + "/" + row.Subdivision + "/" + row.Function
		switch {
		case row.EffectiveAt.After(now):
			matrix.Scheduled = append(matrix.Scheduled, rule)
		case !inForce[key]:
			inForce[key] = true
			matrix.Current = append(matrix.Current, rule)
		default:
			matrix.History = append(matrix.History, rule)
		}
	}
	return matrix, nil
}

// AdminSetMarketRule appends rule by adminUUID, effective now when its
// EffectiveAt is zero. Enabling while operator conditions are noted is
// refused: the admin clears the note once they are met.
func (s *Service) AdminSetMarketRule(ctx context.Context, adminUUID string, rule compliance.MarketRule) (compliance.SetMarketRuleOutcome, error) {
	defer logging.Step(ctx, "compliance.admin_set_market_rule").Close()
	if _, err := uuid.Parse(adminUUID); err != nil {
		return "", fmt.Errorf("invalid admin uuid: %w", err)
	}
	now := s.now()
	if rule.EffectiveAt.IsZero() {
		rule.EffectiveAt = now
	}
	outcome := validate(rule, now)
	if outcome == compliance.MarketRuleRecorded {
		err := s.repo.InsertMarketRule(ctx, internalrepo.MarketRule{
			Country: rule.Country, Subdivision: rule.Subdivision, Function: string(rule.Function), Enabled: rule.Enabled,
			MinAge: rule.MinAge, ConditionsNote: rule.ConditionsNote, EffectiveAt: rule.EffectiveAt,
			CreatedByUserUUID: &adminUUID, Reason: rule.Reason, CreatedAt: now,
		})
		if err != nil {
			return "", err
		}
	}
	setRuleTotal.Inc(ctx, outcomeLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("market_rule", string(outcome)))
	return outcome, nil
}

func validate(rule compliance.MarketRule, now time.Time) compliance.SetMarketRuleOutcome {
	switch {
	case !rules.ValidCountry(rule.Country), !rules.ValidSubdivision(rule.Subdivision), !rules.KnownFunction(rule.Function):
		return compliance.MarketRuleInvalid
	case rule.Reason == "", len(rule.Reason) > maxTextLength, len(rule.ConditionsNote) > maxTextLength:
		return compliance.MarketRuleInvalid
	case rule.EffectiveAt.Before(now.Add(-pastTolerance)):
		return compliance.MarketRuleInvalid
	case rules.AdultFunctions[rule.Function] && (rule.MinAge < 1 || rule.MinAge > maxMinAge):
		return compliance.MarketRuleInvalid
	case !rules.AdultFunctions[rule.Function] && rule.MinAge != 0:
		return compliance.MarketRuleInvalid
	case rule.Enabled && rule.ConditionsNote != "":
		return compliance.MarketRuleConditionsPending
	}
	return compliance.MarketRuleRecorded
}

func toRule(row internalrepo.MarketRule) compliance.MarketRule {
	rule := compliance.MarketRule{
		Country: row.Country, Subdivision: row.Subdivision, Function: compliance.Function(row.Function), Enabled: row.Enabled,
		MinAge: row.MinAge, ConditionsNote: row.ConditionsNote, EffectiveAt: row.EffectiveAt, Reason: row.Reason, CreatedAt: row.CreatedAt,
	}
	if row.CreatedByUserUUID != nil {
		rule.CreatedByUserUUID = *row.CreatedByUserUUID
	}
	return rule
}
