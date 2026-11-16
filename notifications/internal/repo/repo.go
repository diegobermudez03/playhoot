// Package repo persists Notifications: entries, preferences, email
// deliveries, suppressions, the outbound mode and provider usage. Every
// method takes the handle to run on, so callers decide the transaction.
package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Entry is a notifications_entries row.
type Entry struct {
	ID               int64
	EntryUUID        string
	UserUUID         string
	SourceDomain     string
	Type             string
	ExternalToken    string
	IdempotencyKey   string
	Skeleton         string `gorm:"type:jsonb"`
	SendAt           time.Time
	InApp            bool
	EmailChannel     bool
	Necessary        bool
	ReadAt           *time.Time
	ContentClearedAt *time.Time
	CreatedAt        time.Time
}

func (Entry) TableName() string { return "notifications_entries" }

// Preferences is a notifications_preferences row.
type Preferences struct {
	ID             int64
	UserUUID       string
	AIEmail        bool `gorm:"column:ai_email"`
	CapacityEmail  bool
	ReviewEmail    bool
	MarketingEmail bool
	CreatedAt      time.Time
}

func (Preferences) TableName() string { return "notifications_preferences" }

// Delivery is a notifications_email_deliveries row.
type Delivery struct {
	ID                int64
	EntryID           int64
	Provider          string
	State             string
	ProviderMessageID string
	Detail            string
	ClosesEmail       bool
	DecidedAt         *time.Time
	CreatedAt         time.Time
}

func (Delivery) TableName() string { return "notifications_email_deliveries" }

// Suppression is a notifications_suppressions row.
type Suppression struct {
	ID        int64
	Email     string
	Reason    string
	Origin    string
	CreatedAt time.Time
}

func (Suppression) TableName() string { return "notifications_suppressions" }

// ModeChange is a notifications_provider_settings row.
type ModeChange struct {
	ID                int64
	Mode              string
	ChangedByUserUUID *string
	Reason            string
	CreatedAt         time.Time
}

func (ModeChange) TableName() string { return "notifications_provider_settings" }

// Usage is a notifications_provider_usage row.
type Usage struct {
	ID       int64
	Provider string
	Day      time.Time `gorm:"type:date"`
	Sent     int
}

func (Usage) TableName() string { return "notifications_provider_usage" }

// Repo is Notifications' persistence.
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

// InsertEntry stores entry and reports whether it was new: an entry with
// the same source and key already stored makes it a no-op.
func (r *Repo) InsertEntry(ctx context.Context, tx *gorm.DB, entry *Entry) (bool, error) {
	result := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(entry)
	if result.Error != nil {
		return false, fmt.Errorf("inserting entry: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// EntryByKey returns the entry of source and key, nil when none.
func (r *Repo) EntryByKey(ctx context.Context, tx *gorm.DB, source, key string) (*Entry, error) {
	var rows []Entry
	err := tx.WithContext(ctx).Raw(`SELECT * FROM notifications_entries WHERE source_domain = ? AND idempotency_key = ?`, source, key).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading entry by key: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// EntryByUUID returns the entry, nil when none.
func (r *Repo) EntryByUUID(ctx context.Context, tx *gorm.DB, entryUUID string) (*Entry, error) {
	var rows []Entry
	err := tx.WithContext(ctx).Raw(`SELECT * FROM notifications_entries WHERE entry_uuid = ?`, entryUUID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading entry: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// EntryFilter narrows ListEntries; zero values do not filter.
type EntryFilter struct {
	SourceDomain string
	Type         string
	From         time.Time
	To           time.Time
	Before       time.Time
	Limit        int
}

// ListEntries returns userUUID's in-app entries, newest first.
func (r *Repo) ListEntries(ctx context.Context, tx *gorm.DB, userUUID string, filter EntryFilter) ([]Entry, error) {
	conditions := []string{"user_uuid = ?", "in_app"}
	args := []any{userUUID}
	if filter.SourceDomain != "" {
		conditions, args = append(conditions, "source_domain = ?"), append(args, filter.SourceDomain)
	}
	if filter.Type != "" {
		conditions, args = append(conditions, "type = ?"), append(args, filter.Type)
	}
	if !filter.From.IsZero() {
		conditions, args = append(conditions, "created_at >= ?"), append(args, filter.From)
	}
	if !filter.To.IsZero() {
		conditions, args = append(conditions, "created_at < ?"), append(args, filter.To)
	}
	if !filter.Before.IsZero() {
		conditions, args = append(conditions, "created_at < ?"), append(args, filter.Before)
	}
	args = append(args, filter.Limit)
	var rows []Entry
	err := tx.WithContext(ctx).Raw(`SELECT * FROM notifications_entries WHERE `+strings.Join(conditions, " AND ")+
		` ORDER BY created_at DESC, id DESC LIMIT ?`, args...).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing entries: %w", err)
	}
	return rows, nil
}

// UnreadCount counts userUUID's unread in-app entries.
func (r *Repo) UnreadCount(ctx context.Context, tx *gorm.DB, userUUID string) (int, error) {
	var count int
	err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM notifications_entries WHERE user_uuid = ? AND read_at IS NULL AND in_app`, userUUID).Scan(&count).Error
	if err != nil {
		return 0, fmt.Errorf("counting unread entries: %w", err)
	}
	return count, nil
}

// MarkRead sets read_at once on userUUID's entry; false when it was
// already read or is not theirs.
func (r *Repo) MarkRead(ctx context.Context, tx *gorm.DB, userUUID, entryUUID string, at time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`UPDATE notifications_entries SET read_at = ? WHERE entry_uuid = ? AND user_uuid = ? AND read_at IS NULL`, at, entryUUID, userUUID)
	if result.Error != nil {
		return false, fmt.Errorf("marking entry read: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ClearContent empties the skeleton of an entry not shown in Playhoot, once.
func (r *Repo) ClearContent(ctx context.Context, tx *gorm.DB, entryID int64, empty string, at time.Time) error {
	err := tx.WithContext(ctx).Exec(`UPDATE notifications_entries SET skeleton = ?, content_cleared_at = ? WHERE id = ? AND content_cleared_at IS NULL`, empty, at, entryID).Error
	if err != nil {
		return fmt.Errorf("clearing entry content: %w", err)
	}
	return nil
}

// LatestPreferences returns userUUID's current preferences, nil when none.
func (r *Repo) LatestPreferences(ctx context.Context, tx *gorm.DB, userUUID string) (*Preferences, error) {
	var rows []Preferences
	err := tx.WithContext(ctx).Raw(`SELECT * FROM notifications_preferences WHERE user_uuid = ? ORDER BY created_at DESC, id DESC LIMIT 1`, userUUID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading preferences: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// InsertPreferences appends a preference row.
func (r *Repo) InsertPreferences(ctx context.Context, tx *gorm.DB, row *Preferences) error {
	if err := tx.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("inserting preferences: %w", err)
	}
	return nil
}

// DeliveriesForEntry returns every attempt of an entry's email, oldest first.
func (r *Repo) DeliveriesForEntry(ctx context.Context, tx *gorm.DB, entryID int64) ([]Delivery, error) {
	var rows []Delivery
	err := tx.WithContext(ctx).Raw(`SELECT * FROM notifications_email_deliveries WHERE entry_id = ? ORDER BY id`, entryID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading deliveries: %w", err)
	}
	return rows, nil
}

// LockEntry serializes the email attempts of one entry.
func (r *Repo) LockEntry(ctx context.Context, tx *gorm.DB, entryID int64) error {
	if err := tx.WithContext(ctx).Exec(`SELECT id FROM notifications_entries WHERE id = ? FOR UPDATE`, entryID).Error; err != nil {
		return fmt.Errorf("locking entry: %w", err)
	}
	return nil
}

// InsertDelivery stores an attempt.
func (r *Repo) InsertDelivery(ctx context.Context, tx *gorm.DB, row *Delivery) error {
	if err := tx.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("inserting delivery: %w", err)
	}
	return nil
}

// DecideDelivery moves a pending attempt to its final state, once.
func (r *Repo) DecideDelivery(ctx context.Context, tx *gorm.DB, id int64, pending, state, messageID, detail string, closes bool, at time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`UPDATE notifications_email_deliveries SET state = ?, provider_message_id = ?, detail = ?, closes_email = ?, decided_at = ?
		WHERE id = ? AND state = ?`, state, messageID, detail, closes, at, id, pending)
	if result.Error != nil {
		return false, fmt.Errorf("deciding delivery: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// DeleteDeliveriesBefore deletes up to batch attempts created before cutoff.
func (r *Repo) DeleteDeliveriesBefore(ctx context.Context, tx *gorm.DB, cutoff time.Time, batch int) (int64, error) {
	result := tx.WithContext(ctx).Exec(`DELETE FROM notifications_email_deliveries WHERE id IN
		(SELECT id FROM notifications_email_deliveries WHERE created_at < ? ORDER BY created_at LIMIT ?)`, cutoff, batch)
	if result.Error != nil {
		return 0, fmt.Errorf("deleting expired deliveries: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// StateCount is how many attempts are in a state.
type StateCount struct {
	State string
	Count int
}

// CountDeliveriesSince counts attempts created since by state.
func (r *Repo) CountDeliveriesSince(ctx context.Context, tx *gorm.DB, since time.Time) ([]StateCount, error) {
	var rows []StateCount
	err := tx.WithContext(ctx).Raw(`SELECT state, COUNT(*) AS count FROM notifications_email_deliveries WHERE created_at >= ? GROUP BY state`, since).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("counting deliveries: %w", err)
	}
	return rows, nil
}

// SuppressionReasons returns the reasons recorded for email.
func (r *Repo) SuppressionReasons(ctx context.Context, tx *gorm.DB, email string) ([]string, error) {
	var reasons []string
	err := tx.WithContext(ctx).Raw(`SELECT DISTINCT reason FROM notifications_suppressions WHERE email = ?`, email).Scan(&reasons).Error
	if err != nil {
		return nil, fmt.Errorf("reading suppressions: %w", err)
	}
	return reasons, nil
}

// HasSuppression reports whether email, reason and origin are recorded.
func (r *Repo) HasSuppression(ctx context.Context, tx *gorm.DB, email, reason, origin string) (bool, error) {
	var count int
	err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM notifications_suppressions WHERE email = ? AND reason = ? AND origin = ?`, email, reason, origin).Scan(&count).Error
	if err != nil {
		return false, fmt.Errorf("reading suppression: %w", err)
	}
	return count > 0, nil
}

// InsertSuppression appends a suppression.
func (r *Repo) InsertSuppression(ctx context.Context, tx *gorm.DB, row *Suppression) error {
	if err := tx.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("inserting suppression: %w", err)
	}
	return nil
}

// LockSuppressions serializes suppression writes for one address.
func (r *Repo) LockSuppressions(ctx context.Context, tx *gorm.DB, email string) error {
	if err := tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, email).Error; err != nil {
		return fmt.Errorf("locking suppressions: %w", err)
	}
	return nil
}

// LatestMode returns the current outbound mode row.
func (r *Repo) LatestMode(ctx context.Context, tx *gorm.DB) (*ModeChange, error) {
	var rows []ModeChange
	err := tx.WithContext(ctx).Raw(`SELECT * FROM notifications_provider_settings ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading mode: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// InsertMode appends a mode change.
func (r *Repo) InsertMode(ctx context.Context, tx *gorm.DB, row *ModeChange) error {
	if err := tx.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("inserting mode: %w", err)
	}
	return nil
}

// IncrementUsage counts one accepted send of provider on day.
func (r *Repo) IncrementUsage(ctx context.Context, tx *gorm.DB, provider string, day time.Time) error {
	err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}, {Name: "day"}},
		DoUpdates: clause.Assignments(map[string]any{"sent": gorm.Expr("notifications_provider_usage.sent + EXCLUDED.sent")}),
	}).Create(&Usage{Provider: provider, Day: day, Sent: 1}).Error
	if err != nil {
		return fmt.Errorf("counting usage: %w", err)
	}
	return nil
}

// UsageBetween sums provider's accepted sends on days in [from, to).
func (r *Repo) UsageBetween(ctx context.Context, tx *gorm.DB, provider string, from, to time.Time) (int, error) {
	var sent int
	err := tx.WithContext(ctx).Raw(`SELECT COALESCE(SUM(sent), 0) FROM notifications_provider_usage WHERE provider = ? AND day >= ? AND day < ?`, provider, from, to).Scan(&sent).Error
	if err != nil {
		return 0, fmt.Errorf("reading usage: %w", err)
	}
	return sent, nil
}
