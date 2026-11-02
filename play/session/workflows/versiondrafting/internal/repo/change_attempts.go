package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
)

// ChangeAttempt is one change sent to a version under an idempotency key.
// Outcome, Revision and Diagnostics are nil until it is completed.
type ChangeAttempt struct {
	ID            uint
	RequestSHA256 string
	BaseRevision  int64
	Origin        string
	Outcome       *string
	Revision      *int64
	Diagnostics   []draftsources.Diagnostic
	CreatedAt     time.Time
}

type changeAttemptRow struct {
	ID            uint
	RequestSHA256 string `gorm:"column:request_sha256"`
	BaseRevision  int64
	Origin        string
	Outcome       *string
	Revision      *int64
	Diagnostics   []byte
	CreatedAt     time.Time
}

// changeAttemptClaimRow is the columns a change attempt has when claimed.
type changeAttemptClaimRow struct {
	ID             uint
	GameVersionID  uint
	IdempotencyKey string
	RequestSHA256  string `gorm:"column:request_sha256"`
	BaseRevision   int64
	Origin         string
	CreatedAt      time.Time
}

func (changeAttemptClaimRow) TableName() string { return "play_game_version_change_attempts" }

func (row changeAttemptRow) attempt() (ChangeAttempt, error) {
	attempt := ChangeAttempt{
		ID: row.ID, RequestSHA256: row.RequestSHA256, BaseRevision: row.BaseRevision, Origin: row.Origin,
		Outcome: row.Outcome, Revision: row.Revision, CreatedAt: row.CreatedAt,
	}
	if len(row.Diagnostics) > 0 {
		if err := json.Unmarshal(row.Diagnostics, &attempt.Diagnostics); err != nil {
			return ChangeAttempt{}, fmt.Errorf("decoding attempt diagnostics: %s", err)
		}
	}
	return attempt, nil
}

const selectChangeAttempt = `
	SELECT id, request_sha256, base_revision, origin, outcome, revision, diagnostics, created_at
	FROM play_game_version_change_attempts
`

// ClaimChangeAttempt claims (versionID, idempotencyKey), or returns the
// attempt that already holds it. A concurrent claim of the same key waits for
// the first transaction and then sees its attempt.
func (r *Repo) ClaimChangeAttempt(ctx context.Context, tx *gorm.DB, versionID uint, idempotencyKey string, requestSHA256 string, baseRevision int64, origin string) (uint, *ChangeAttempt, error) {
	claimRow := changeAttemptClaimRow{GameVersionID: versionID, IdempotencyKey: idempotencyKey, RequestSHA256: requestSHA256, BaseRevision: baseRevision, Origin: origin}
	claim := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&claimRow)
	if claim.Error != nil {
		return 0, nil, fmt.Errorf("claiming change attempt: %s", claim.Error)
	}
	if claim.RowsAffected == 1 {
		return claimRow.ID, nil, nil
	}
	var row changeAttemptRow
	if err := tx.WithContext(ctx).Raw(selectChangeAttempt+`WHERE game_version_id = ? AND idempotency_key = ?`,
		versionID, idempotencyKey).Scan(&row).Error; err != nil {
		return 0, nil, fmt.Errorf("reading existing change attempt: %s", err)
	}
	existing, err := row.attempt()
	if err != nil {
		return 0, nil, err
	}
	return 0, &existing, nil
}

// CompleteChangeAttempt records what attemptID decided, once.
func (r *Repo) CompleteChangeAttempt(ctx context.Context, tx *gorm.DB, attemptID uint, outcome string, revision *int64, diagnostics []draftsources.Diagnostic) error {
	var encoded any
	if len(diagnostics) > 0 {
		bytes, err := json.Marshal(diagnostics)
		if err != nil {
			return fmt.Errorf("encoding attempt diagnostics: %s", err)
		}
		encoded = string(bytes)
	}
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_game_version_change_attempts SET outcome = ?, revision = ?, diagnostics = ?
		WHERE id = ? AND outcome IS NULL
	`, outcome, revision, encoded, attemptID)
	if result.Error != nil {
		return fmt.Errorf("completing change attempt: %s", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("change attempt %d was not open", attemptID)
	}
	return nil
}

// RecentChangeAttempts returns versionID's completed attempts, newest first,
// at most limit.
func (r *Repo) RecentChangeAttempts(ctx context.Context, tx *gorm.DB, versionID uint, limit int) ([]ChangeAttempt, error) {
	var rows []changeAttemptRow
	if err := tx.WithContext(ctx).Raw(selectChangeAttempt+`
		WHERE game_version_id = ? AND outcome IS NOT NULL ORDER BY id DESC LIMIT ?
	`, versionID, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing change attempts: %s", err)
	}
	attempts := make([]ChangeAttempt, 0, len(rows))
	for _, row := range rows {
		attempt, err := row.attempt()
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, nil
}
