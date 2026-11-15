// Package repo persists Budget: rates, caps, spend events, reservations and
// the restriction ladder changes. Every method takes the handle to run on,
// so callers decide the transaction.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// admissionLockKey serializes admissions and ladder moves across instances.
const admissionLockKey int64 = 7_210_550_001

// Rate is a budget_rates row.
type Rate struct {
	ID                int64
	Category          string
	Service           string
	Unit              string
	USDPerUnit        float64
	EffectiveAt       time.Time
	CreatedByUserUUID *string
	Reason            string
	CreatedAt         time.Time
}

func (Rate) TableName() string { return "budget_rates" }

// Cap is a budget_caps row.
type Cap struct {
	ID                int64
	Scope             string
	CapWindow         string
	USDLimit          float64
	MaxConcurrency    *int
	MaxVolume         *int
	EffectiveAt       time.Time
	CreatedByUserUUID *string
	Reason            string
	CreatedAt         time.Time
}

func (Cap) TableName() string { return "budget_caps" }

// SpendEvent is a budget_spend_events row.
type SpendEvent struct {
	ID             int64
	SourceDomain   string
	Category       string
	Service        string
	Reference      string
	Quantity       float64
	Unit           string
	USD            float64 `gorm:"column:usd"`
	Measured       bool
	RateMissing    bool
	IdempotencyKey string
	OccurredAt     time.Time
	CreatedAt      time.Time
}

func (SpendEvent) TableName() string { return "budget_spend_events" }

// Reservation is a budget_reservations row.
type Reservation struct {
	ID              int64
	ReservationUUID string
	Category        string
	Reference       string
	USDReserved     float64  `gorm:"column:usd_reserved"`
	SettledUSD      *float64 `gorm:"column:settled_usd"`
	ReleasedAt      *time.Time
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

func (Reservation) TableName() string { return "budget_reservations" }

// Change is a budget_restriction_changes row; the category lists are JSON
// arrays.
type Change struct {
	ID               int64
	Level            string
	PausedCategories string
	ReopenClosed     string
	Reason           string
	Actor            string
	CreatedAt        time.Time
}

func (Change) TableName() string { return "budget_restriction_changes" }

// Repo is Budget's persistence.
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

// Lock takes the transaction-scoped lock every admission and ladder move
// holds, so concurrent ones never decide on the same remainder.
func (r *Repo) Lock(ctx context.Context, tx *gorm.DB) error {
	if err := tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(?)`, admissionLockKey).Error; err != nil {
		return fmt.Errorf("locking budget admission: %s", err)
	}
	return nil
}

const rateColumns = `id, category, service, unit, usd_per_unit, effective_at, created_by_user_uuid, reason, created_at`

// RatesAt returns the rate in force at at for every service.
func (r *Repo) RatesAt(ctx context.Context, db *gorm.DB, at time.Time) ([]Rate, error) {
	var rows []Rate
	err := db.WithContext(ctx).Raw(`SELECT DISTINCT ON (category, service) `+rateColumns+` FROM budget_rates
		WHERE effective_at <= ? ORDER BY category, service, effective_at DESC, id DESC`, at).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading rates: %s", err)
	}
	return rows, nil
}

// RateFor returns the rate of service in category in force at at, or nil.
func (r *Repo) RateFor(ctx context.Context, db *gorm.DB, category, service string, at time.Time) (*Rate, error) {
	var rows []Rate
	err := db.WithContext(ctx).Raw(`SELECT `+rateColumns+` FROM budget_rates
		WHERE category = ? AND service = ? AND effective_at <= ? ORDER BY effective_at DESC, id DESC LIMIT 1`,
		category, service, at).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading rate: %s", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// CapsAt returns the cap in force at at for every scope and window.
func (r *Repo) CapsAt(ctx context.Context, db *gorm.DB, at time.Time) ([]Cap, error) {
	var rows []Cap
	err := db.WithContext(ctx).Raw(`SELECT DISTINCT ON (scope, cap_window) id, scope, cap_window, usd_limit, max_concurrency,
		max_volume, effective_at, created_by_user_uuid, reason, created_at FROM budget_caps
		WHERE effective_at <= ? ORDER BY scope, cap_window, effective_at DESC, id DESC`, at).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading caps: %s", err)
	}
	return rows, nil
}

// InsertControls appends caps and rates in one statement each.
func (r *Repo) InsertControls(ctx context.Context, db *gorm.DB, caps []Cap, rates []Rate) error {
	if len(caps) > 0 {
		if err := db.WithContext(ctx).Create(&caps).Error; err != nil {
			return fmt.Errorf("inserting caps: %s", err)
		}
	}
	if len(rates) > 0 {
		if err := db.WithContext(ctx).Create(&rates).Error; err != nil {
			return fmt.Errorf("inserting rates: %s", err)
		}
	}
	return nil
}

// InsertSpendEvent stores event unless its source and key were stored
// before; it reports whether it was new.
func (r *Repo) InsertSpendEvent(ctx context.Context, db *gorm.DB, event SpendEvent) (bool, error) {
	result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
	if result.Error != nil {
		return false, fmt.Errorf("inserting spend event: %s", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// SpendTotal is one category's spend from the month start: per window, and
// over the trailing interval.
type SpendTotal struct {
	Category string
	Month    float64
	Day      float64
	Hour     float64
	Trailing float64
	// MeasuredMonth and EstimatedMonth split Month.
	MeasuredMonth  float64
	EstimatedMonth float64
	MeasuredDay    float64
	MeasuredHour   float64
}

// SpendTotals sums spend per category from monthStart. An estimate stops
// counting once a measured event of the same category names its reference.
func (r *Repo) SpendTotals(ctx context.Context, db *gorm.DB, categories []string, monthStart, dayStart, hourStart, trailingStart time.Time) ([]SpendTotal, error) {
	var rows []SpendTotal
	err := db.WithContext(ctx).Raw(`SELECT category,
			COALESCE(SUM(usd), 0) AS month,
			COALESCE(SUM(usd) FILTER (WHERE occurred_at >= ?), 0) AS day,
			COALESCE(SUM(usd) FILTER (WHERE occurred_at >= ?), 0) AS hour,
			COALESCE(SUM(usd) FILTER (WHERE occurred_at >= ?), 0) AS trailing,
			COALESCE(SUM(usd) FILTER (WHERE measured), 0) AS measured_month,
			COALESCE(SUM(usd) FILTER (WHERE NOT measured), 0) AS estimated_month,
			COALESCE(SUM(usd) FILTER (WHERE measured AND occurred_at >= ?), 0) AS measured_day,
			COALESCE(SUM(usd) FILTER (WHERE measured AND occurred_at >= ?), 0) AS measured_hour
		FROM budget_spend_events e
		WHERE e.category IN ? AND e.occurred_at >= ?
			AND (e.measured OR e.reference = '' OR NOT EXISTS (
				SELECT 1 FROM budget_spend_events m
				WHERE m.category = e.category AND m.reference = e.reference AND m.measured))
		GROUP BY category`,
		dayStart, hourStart, trailingStart, dayStart, hourStart, categories, monthStart).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("summing spend: %s", err)
	}
	return rows, nil
}

// ReservationTotal is one category's admitted work: active reservations and
// admissions started per window.
type ReservationTotal struct {
	Category     string
	Reserved     float64
	Active       int
	StartedMonth int
	StartedDay   int
	StartedHour  int
}

// ReservationTotals sums active reservations at now and counts admissions
// started since each window start, per category.
func (r *Repo) ReservationTotals(ctx context.Context, db *gorm.DB, categories []string, now, monthStart, dayStart, hourStart time.Time) ([]ReservationTotal, error) {
	var active []ReservationTotal
	err := db.WithContext(ctx).Raw(`SELECT category, COALESCE(SUM(usd_reserved), 0) AS reserved, COUNT(*) AS active
		FROM budget_reservations WHERE category IN ? AND released_at IS NULL AND expires_at > ?
		GROUP BY category`, categories, now).Scan(&active).Error
	if err != nil {
		return nil, fmt.Errorf("summing reservations: %s", err)
	}
	var started []ReservationTotal
	err = db.WithContext(ctx).Raw(`SELECT category, COUNT(*) AS started_month,
			COUNT(*) FILTER (WHERE created_at >= ?) AS started_day,
			COUNT(*) FILTER (WHERE created_at >= ?) AS started_hour
		FROM budget_reservations WHERE category IN ? AND created_at >= ?
		GROUP BY category`, dayStart, hourStart, categories, monthStart).Scan(&started).Error
	if err != nil {
		return nil, fmt.Errorf("counting admissions: %s", err)
	}
	byCategory := map[string]ReservationTotal{}
	for _, row := range append(active, started...) {
		t := byCategory[row.Category]
		t.Category = row.Category
		t.Reserved += row.Reserved
		t.Active += row.Active
		t.StartedMonth += row.StartedMonth
		t.StartedDay += row.StartedDay
		t.StartedHour += row.StartedHour
		byCategory[row.Category] = t
	}
	out := make([]ReservationTotal, 0, len(byCategory))
	for _, t := range byCategory {
		out = append(out, t)
	}
	return out, nil
}

// RateMissingCategories returns the categories with a spend event since
// from that had no rate and whose service still has none at at.
func (r *Repo) RateMissingCategories(ctx context.Context, db *gorm.DB, categories []string, from, at time.Time) ([]string, error) {
	var rows []string
	err := db.WithContext(ctx).Raw(`SELECT DISTINCT e.category FROM budget_spend_events e
		WHERE e.category IN ? AND e.occurred_at >= ? AND e.rate_missing
			AND NOT EXISTS (SELECT 1 FROM budget_rates r
				WHERE r.category = e.category AND r.service = e.service AND r.effective_at <= ?)
		ORDER BY e.category`, categories, from, at).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading missing rates: %s", err)
	}
	return rows, nil
}

// InsertReservation stores an admission.
func (r *Repo) InsertReservation(ctx context.Context, db *gorm.DB, reservation Reservation) error {
	if err := db.WithContext(ctx).Create(&reservation).Error; err != nil {
		return fmt.Errorf("inserting reservation: %s", err)
	}
	return nil
}

// ReservationForUpdate returns a reservation locked for this transaction,
// or nil.
func (r *Repo) ReservationForUpdate(ctx context.Context, tx *gorm.DB, reservationUUID string) (*Reservation, error) {
	var rows []Reservation
	err := tx.WithContext(ctx).Raw(`SELECT id, reservation_uuid, category, reference, usd_reserved, settled_usd,
		released_at, expires_at, created_at FROM budget_reservations WHERE reservation_uuid = ? FOR UPDATE`,
		reservationUUID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading reservation: %s", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// ReleaseReservation closes a reservation once with settledUSD.
func (r *Repo) ReleaseReservation(ctx context.Context, tx *gorm.DB, reservationUUID string, settledUSD float64, at time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`UPDATE budget_reservations SET settled_usd = ?, released_at = ?
		WHERE reservation_uuid = ? AND released_at IS NULL`, settledUSD, at, reservationUUID)
	if result.Error != nil {
		return false, fmt.Errorf("releasing reservation: %s", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ReleaseExpired releases up to limit reservations expired at now, leaving
// settled_usd empty; it answers how many.
func (r *Repo) ReleaseExpired(ctx context.Context, db *gorm.DB, categories []string, now time.Time, limit int) (int64, error) {
	result := db.WithContext(ctx).Exec(`UPDATE budget_reservations SET released_at = ?
		WHERE id IN (SELECT id FROM budget_reservations
			WHERE category IN ? AND released_at IS NULL AND expires_at <= ? LIMIT ?)`,
		now, categories, now, limit)
	if result.Error != nil {
		return 0, fmt.Errorf("releasing expired reservations: %s", result.Error)
	}
	return result.RowsAffected, nil
}

// LatestChange returns the ladder's current row, or nil before any.
func (r *Repo) LatestChange(ctx context.Context, db *gorm.DB) (*Change, error) {
	var rows []Change
	err := db.WithContext(ctx).Raw(`SELECT id, level, paused_categories::text AS paused_categories,
		reopen_closed::text AS reopen_closed, reason, actor, created_at
		FROM budget_restriction_changes ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading ladder: %s", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// InsertChange appends a ladder move and returns its id.
func (r *Repo) InsertChange(ctx context.Context, db *gorm.DB, change Change) (int64, error) {
	if err := db.WithContext(ctx).Create(&change).Error; err != nil {
		return 0, fmt.Errorf("inserting ladder change: %s", err)
	}
	return change.ID, nil
}
