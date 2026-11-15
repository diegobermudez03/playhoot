// Package repo persists Compliance: the market matrix, declarations,
// minority signals, acceptance texts and acceptances, all append-only.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is Compliance's persistence.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB returns the database, for transactions.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}

// noSubdivision is the subdivision of a rule or declaration for a whole
// country.
const noSubdivision = ""

// LockPerson serializes, until tx ends, the writes of one kind about one
// person.
func (r *Repo) LockPerson(ctx context.Context, tx *gorm.DB, kind string, userUUID string) error {
	if err := tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, kind+":"+userUUID).Error; err != nil {
		return fmt.Errorf("locking person: %s", err)
	}
	return nil
}

// MarketRule is a compliance_market_rules row.
type MarketRule struct {
	ID                int64
	Country           string
	Subdivision       string
	Function          string
	Enabled           bool
	MinAge            int
	ConditionsNote    string
	EffectiveAt       time.Time
	CreatedByUserUUID *string
	Reason            string
	CreatedAt         time.Time
}

func (MarketRule) TableName() string { return "compliance_market_rules" }

const ruleColumns = `id, country, subdivision, function, enabled, min_age, conditions_note, effective_at, created_by_user_uuid, reason, created_at`

// CurrentRule returns the rule in force at now for a place and function, or
// nil.
func (r *Repo) CurrentRule(ctx context.Context, db *gorm.DB, country, subdivision, function string, now time.Time) (*MarketRule, error) {
	var rules []MarketRule
	err := db.WithContext(ctx).Raw(`SELECT `+ruleColumns+` FROM compliance_market_rules
		WHERE country = ? AND subdivision = ? AND function = ? AND effective_at <= ?
		ORDER BY effective_at DESC, id DESC LIMIT 1`, country, subdivision, function, now).Scan(&rules).Error
	if err != nil {
		return nil, fmt.Errorf("reading market rule: %s", err)
	}
	if len(rules) == 0 {
		return nil, nil
	}
	return &rules[0], nil
}

// PlaceRules returns the rules in force at now for every function of a
// place.
func (r *Repo) PlaceRules(ctx context.Context, db *gorm.DB, country, subdivision string, now time.Time) ([]MarketRule, error) {
	var rules []MarketRule
	err := db.WithContext(ctx).Raw(`SELECT DISTINCT ON (function) `+ruleColumns+` FROM compliance_market_rules
		WHERE country = ? AND subdivision = ? AND effective_at <= ?
		ORDER BY function, effective_at DESC, id DESC`, country, subdivision, now).Scan(&rules).Error
	if err != nil {
		return nil, fmt.Errorf("reading place rules: %s", err)
	}
	return rules, nil
}

// CountryCoverage reports whether the matrix has any rule in force at now
// for country, and whether any of them is per state or province.
func (r *Repo) CountryCoverage(ctx context.Context, db *gorm.DB, country string, now time.Time) (covered bool, perSubdivision bool, err error) {
	var row struct {
		Covered        bool
		PerSubdivision bool
	}
	err = db.WithContext(ctx).Raw(`SELECT
			EXISTS (SELECT 1 FROM compliance_market_rules WHERE country = ? AND effective_at <= ?) AS covered,
			EXISTS (SELECT 1 FROM compliance_market_rules WHERE country = ? AND subdivision <> ? AND effective_at <= ?) AS per_subdivision`,
		country, now, country, noSubdivision, now).Scan(&row).Error
	if err != nil {
		return false, false, fmt.Errorf("reading country coverage: %s", err)
	}
	return row.Covered, row.PerSubdivision, nil
}

// MarketRules returns every rule, of country when not empty, newest first per
// place and function.
func (r *Repo) MarketRules(ctx context.Context, country string) ([]MarketRule, error) {
	var rules []MarketRule
	query := `SELECT ` + ruleColumns + ` FROM compliance_market_rules
		ORDER BY country, subdivision, function, effective_at DESC, id DESC`
	args := []any{}
	if country != "" {
		query = `SELECT ` + ruleColumns + ` FROM compliance_market_rules WHERE country = ?
			ORDER BY country, subdivision, function, effective_at DESC, id DESC`
		args = append(args, country)
	}
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rules).Error; err != nil {
		return nil, fmt.Errorf("reading market rules: %s", err)
	}
	return rules, nil
}

// InsertMarketRule appends a rule.
func (r *Repo) InsertMarketRule(ctx context.Context, rule MarketRule) error {
	if err := r.db.WithContext(ctx).Create(&rule).Error; err != nil {
		return fmt.Errorf("inserting market rule: %s", err)
	}
	return nil
}

// Declaration is a compliance_declarations row.
type Declaration struct {
	ID           int64
	UserUUID     string
	Country      string
	Subdivision  string
	MeetsAge     bool
	ThresholdAge int
	Origin       string
	CreatedAt    time.Time
}

func (Declaration) TableName() string { return "compliance_declarations" }

// LatestDeclaration returns the person's current declaration, or nil.
func (r *Repo) LatestDeclaration(ctx context.Context, db *gorm.DB, userUUID string) (*Declaration, error) {
	var declarations []Declaration
	err := db.WithContext(ctx).Raw(`SELECT id, user_uuid, country, subdivision, meets_age, threshold_age, origin, created_at
		FROM compliance_declarations WHERE user_uuid = ? ORDER BY created_at DESC, id DESC LIMIT 1`, userUUID).Scan(&declarations).Error
	if err != nil {
		return nil, fmt.Errorf("reading declaration: %s", err)
	}
	if len(declarations) == 0 {
		return nil, nil
	}
	return &declarations[0], nil
}

// InsertDeclaration appends a declaration.
func (r *Repo) InsertDeclaration(ctx context.Context, tx *gorm.DB, declaration Declaration) error {
	if err := tx.WithContext(ctx).Create(&declaration).Error; err != nil {
		return fmt.Errorf("inserting declaration: %s", err)
	}
	return nil
}

// MinoritySignal is a compliance_minority_signals row.
type MinoritySignal struct {
	ID              int64
	UserUUID        string
	Origin          string
	NotedByUserUUID *string
	CreatedAt       time.Time
}

func (MinoritySignal) TableName() string { return "compliance_minority_signals" }

// HasMinoritySignal reports whether the person is known to be a minor.
func (r *Repo) HasMinoritySignal(ctx context.Context, db *gorm.DB, userUUID string) (bool, error) {
	var known bool
	err := db.WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM compliance_minority_signals WHERE user_uuid = ?)`, userUUID).Scan(&known).Error
	if err != nil {
		return false, fmt.Errorf("reading minority signals: %s", err)
	}
	return known, nil
}

// InsertMinoritySignal appends a minority signal.
func (r *Repo) InsertMinoritySignal(ctx context.Context, tx *gorm.DB, signal MinoritySignal) error {
	if err := tx.WithContext(ctx).Create(&signal).Error; err != nil {
		return fmt.Errorf("inserting minority signal: %s", err)
	}
	return nil
}

// AcceptanceText is a compliance_acceptance_texts row.
type AcceptanceText struct {
	ID          int64
	Kind        string
	Version     int
	Language    string
	Content     string
	PublishedAt time.Time
	CreatedAt   time.Time
}

func (AcceptanceText) TableName() string { return "compliance_acceptance_texts" }

const textColumns = `id, kind, version, language, content, published_at, created_at`

// LatestText returns the newest version of kind in language, or nil.
func (r *Repo) LatestText(ctx context.Context, kind, language string) (*AcceptanceText, error) {
	return r.text(ctx, `SELECT `+textColumns+` FROM compliance_acceptance_texts
		WHERE kind = ? AND language = ? ORDER BY version DESC LIMIT 1`, kind, language)
}

// Text returns the text of kind, version and language, or nil.
func (r *Repo) Text(ctx context.Context, kind string, version int, language string) (*AcceptanceText, error) {
	return r.text(ctx, `SELECT `+textColumns+` FROM compliance_acceptance_texts
		WHERE kind = ? AND version = ? AND language = ?`, kind, version, language)
}

func (r *Repo) text(ctx context.Context, query string, args ...any) (*AcceptanceText, error) {
	var texts []AcceptanceText
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&texts).Error; err != nil {
		return nil, fmt.Errorf("reading acceptance text: %s", err)
	}
	if len(texts) == 0 {
		return nil, nil
	}
	return &texts[0], nil
}

// LatestVersion returns the newest version of kind in any language, 0 when
// there is none.
func (r *Repo) LatestVersion(ctx context.Context, kind string) (int, error) {
	var version int
	err := r.db.WithContext(ctx).Raw(`SELECT COALESCE(MAX(version), 0) FROM compliance_acceptance_texts WHERE kind = ?`, kind).Scan(&version).Error
	if err != nil {
		return 0, fmt.Errorf("reading latest text version: %s", err)
	}
	return version, nil
}

// Acceptance is a compliance_acceptances row.
type Acceptance struct {
	ID        int64
	UserUUID  string
	TextID    int64
	Kind      string
	Action    string
	Context   string
	CreatedAt time.Time
}

func (Acceptance) TableName() string { return "compliance_acceptances" }

// LastAction is the person's newest acceptance or withdrawal of a kind, with
// the version it was about.
type LastAction struct {
	TextID  int64
	Action  string
	Version int
}

// LastAcceptanceAction returns the person's newest action on kind, or nil.
func (r *Repo) LastAcceptanceAction(ctx context.Context, db *gorm.DB, userUUID, kind string) (*LastAction, error) {
	var actions []LastAction
	err := db.WithContext(ctx).Raw(`SELECT a.text_id, a.action, t.version FROM compliance_acceptances a
		JOIN compliance_acceptance_texts t ON t.id = a.text_id
		WHERE a.user_uuid = ? AND a.kind = ? ORDER BY a.created_at DESC, a.id DESC LIMIT 1`, userUUID, kind).Scan(&actions).Error
	if err != nil {
		return nil, fmt.Errorf("reading last acceptance: %s", err)
	}
	if len(actions) == 0 {
		return nil, nil
	}
	return &actions[0], nil
}

// InsertAcceptance appends an acceptance or withdrawal.
func (r *Repo) InsertAcceptance(ctx context.Context, tx *gorm.DB, acceptance Acceptance) error {
	if err := tx.WithContext(ctx).Create(&acceptance).Error; err != nil {
		return fmt.Errorf("inserting acceptance: %s", err)
	}
	return nil
}
