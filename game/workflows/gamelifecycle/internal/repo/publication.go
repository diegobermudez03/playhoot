package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OwnedGame is a Game not deleted, with what its lifecycle steps decide on.
type OwnedGame struct {
	ID          uint
	UUID        string
	OwnerUUID   string
	Name        string
	Visibility  string
	PublishedAt *time.Time
}

// LockGame locks gameUUID's row until tx ends and returns it, or nil when
// there is no such Game or it was deleted.
func (r *Repo) LockGame(ctx context.Context, tx *gorm.DB, gameUUID string) (*OwnedGame, error) {
	var found []OwnedGame
	if err := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, owner_uuid, name, visibility, published_at
		FROM games WHERE uuid = ? AND deleted_at IS NULL
		FOR UPDATE
	`, gameUUID).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("locking game: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// Edition is one edition of a Game.
type Edition struct {
	ID   uint
	UUID string
}

// FindLatestEditionInStatus returns gameID's newest edition in status, or nil.
func (r *Repo) FindLatestEditionInStatus(ctx context.Context, tx *gorm.DB, gameID uint, status string) (*Edition, error) {
	var found []Edition
	if err := tx.WithContext(ctx).Raw(`
		SELECT id, uuid FROM game_editions WHERE game_id = ? AND status = ? ORDER BY id DESC LIMIT 1
	`, gameID, status).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding edition: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// ReleaseRecord is a release with the Game and edition it publishes.
type ReleaseRecord struct {
	ID             uint
	UUID           string
	GameID         uint
	GameUUID       string
	EditionID      uint
	EditionUUID    string
	Sequence       int64
	Revision       int64
	Visibility     string
	IdempotencyKey string
	CompletedAt    *time.Time
	AbandonedAt    *time.Time
}

const selectRelease = `
	SELECT r.id, r.uuid, r.game_id, g.uuid AS game_uuid, r.edition_id, e.uuid AS edition_uuid,
		r.sequence, r.revision, r.visibility, r.idempotency_key, r.completed_at, r.abandoned_at
	FROM game_releases r
	JOIN games g ON g.id = r.game_id
	JOIN game_editions e ON e.id = r.edition_id
`

func (r *Repo) findRelease(ctx context.Context, tx *gorm.DB, where string, args ...any) (*ReleaseRecord, error) {
	var found []ReleaseRecord
	if err := tx.WithContext(ctx).Raw(selectRelease+where, args...).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding release: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// FindReleaseByKey returns gameID's release claimed with idempotencyKey, or nil.
func (r *Repo) FindReleaseByKey(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string) (*ReleaseRecord, error) {
	return r.findRelease(ctx, tx, `WHERE r.game_id = ? AND r.idempotency_key = ?`, gameID, idempotencyKey)
}

// FindPendingRelease returns gameID's release still being published, or nil.
func (r *Repo) FindPendingRelease(ctx context.Context, tx *gorm.DB, gameID uint) (*ReleaseRecord, error) {
	return r.findRelease(ctx, tx, `WHERE r.game_id = ? AND r.completed_at IS NULL AND r.abandoned_at IS NULL`, gameID)
}

// LockRelease locks releaseUUID's row until tx ends and returns it, or nil.
func (r *Repo) LockRelease(ctx context.Context, tx *gorm.DB, releaseUUID string) (*ReleaseRecord, error) {
	return r.findRelease(ctx, tx, `WHERE r.uuid = ? FOR UPDATE OF r`, releaseUUID)
}

// NextReleaseSequence is the sequence gameID's next release takes.
func (r *Repo) NextReleaseSequence(ctx context.Context, tx *gorm.DB, gameID uint) (int64, error) {
	var last []int64
	if err := tx.WithContext(ctx).Raw(`
		SELECT sequence FROM game_releases WHERE game_id = ? ORDER BY sequence DESC LIMIT 1
	`, gameID).Scan(&last).Error; err != nil {
		return 0, fmt.Errorf("finding release sequence: %s", err)
	}
	if len(last) == 0 {
		return 1, nil
	}
	return last[0] + 1, nil
}

// NewRelease is a release about to be recorded.
type NewRelease struct {
	UUID           string
	GameID         uint
	EditionID      uint
	Sequence       int64
	Revision       int64
	Visibility     string
	OwnerUUID      string
	IdempotencyKey string
	CreatedAt      time.Time
}

type releaseRow struct {
	ID             uint
	UUID           string
	GameID         uint
	EditionID      uint
	Sequence       int64
	Revision       int64
	Visibility     string
	OwnerUUID      string
	IdempotencyKey string
	CreatedAt      time.Time
}

func (releaseRow) TableName() string { return "game_releases" }

// CreateRelease records release, being published.
func (r *Repo) CreateRelease(ctx context.Context, tx *gorm.DB, release NewRelease) error {
	row := releaseRow{
		UUID: release.UUID, GameID: release.GameID, EditionID: release.EditionID, Sequence: release.Sequence,
		Revision: release.Revision, Visibility: release.Visibility, OwnerUUID: release.OwnerUUID,
		IdempotencyKey: release.IdempotencyKey, CreatedAt: release.CreatedAt,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("recording release: %s", err)
	}
	return nil
}

// CompleteRelease writes releaseID's completion once; false means it had
// already ended.
func (r *Repo) CompleteRelease(ctx context.Context, tx *gorm.DB, releaseID uint, at time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE game_releases SET completed_at = ? WHERE id = ? AND completed_at IS NULL AND abandoned_at IS NULL
	`, at, releaseID)
	if result.Error != nil {
		return false, fmt.Errorf("completing release: %s", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// AbandonRelease writes releaseID's abandonment once, with why; false means
// it had already ended.
func (r *Repo) AbandonRelease(ctx context.Context, tx *gorm.DB, releaseID uint, reason string, at time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE game_releases SET abandoned_at = ?, abandon_reason = ? WHERE id = ? AND completed_at IS NULL AND abandoned_at IS NULL
	`, at, reason, releaseID)
	if result.Error != nil {
		return false, fmt.Errorf("abandoning release: %s", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ChangeEditionStatus moves editionID from from to to, writing published_at
// when it is the published status, with the status change; false means it
// was no longer in from.
func (r *Repo) ChangeEditionStatus(ctx context.Context, tx *gorm.DB, editionID uint, from string, to string, publishedAt *time.Time, at time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE game_editions SET status = ?, published_at = ? WHERE id = ? AND status = ?
	`, to, publishedAt, editionID, from)
	if result.Error != nil {
		return false, fmt.Errorf("changing edition status: %s", result.Error)
	}
	if result.RowsAffected != 1 {
		return false, nil
	}
	change := editionStatusChangeRow{EditionID: editionID, FromStatus: &from, ToStatus: to, ChangedAt: at}
	if err := tx.WithContext(ctx).Create(&change).Error; err != nil {
		return false, fmt.Errorf("recording edition status change: %s", err)
	}
	return true, nil
}

// MarkPublished records that gameID was published with visibility at: its
// first publication time once, its visibility, and the history of what
// changed (game_histories keeps only changed fields).
func (r *Repo) MarkPublished(ctx context.Context, tx *gorm.DB, game OwnedGame, visibility string, at time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE games SET published_at = ? WHERE id = ? AND published_at IS NULL
	`, at, game.ID).Error; err != nil {
		return fmt.Errorf("recording first publication: %s", err)
	}
	if err := tx.WithContext(ctx).Exec(`
		UPDATE games SET visibility = ?, updated_at = ? WHERE id = ?
	`, visibility, at, game.ID).Error; err != nil {
		return fmt.Errorf("recording visibility: %s", err)
	}
	history := gameHistoryRow{GameID: game.ID, CreatedAt: at}
	changed := false
	if game.PublishedAt == nil {
		published := true
		history.IsPublished = &published
		changed = true
	}
	if game.Visibility != visibility {
		history.Visibility = &visibility
		changed = true
	}
	if !changed {
		return nil
	}
	if err := tx.WithContext(ctx).Create(&history).Error; err != nil {
		return fmt.Errorf("recording publication history: %s", err)
	}
	return nil
}

// EditionRequest is a claimed request to open a new edition; EditionID is
// nil until the opening that claimed it completes it.
type EditionRequest struct {
	ID        uint
	EditionID *uint
}

type editionRequestRow struct {
	ID             uint
	GameID         uint
	IdempotencyKey string
	CreatedAt      time.Time
}

func (editionRequestRow) TableName() string { return "game_edition_requests" }

// FindEditionRequest returns gameID's request holding idempotencyKey, or nil.
func (r *Repo) FindEditionRequest(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string) (*EditionRequest, error) {
	var found []EditionRequest
	if err := tx.WithContext(ctx).Raw(`
		SELECT id, edition_id FROM game_edition_requests WHERE game_id = ? AND idempotency_key = ?
	`, gameID, idempotencyKey).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding edition request: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// ClaimEditionRequest records a request to open an edition with
// idempotencyKey and returns its id.
func (r *Repo) ClaimEditionRequest(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string, at time.Time) (uint, error) {
	row := editionRequestRow{GameID: gameID, IdempotencyKey: idempotencyKey, CreatedAt: at}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("claiming edition request: %s", err)
	}
	if row.ID == 0 {
		return 0, fmt.Errorf("edition request was claimed meanwhile")
	}
	return row.ID, nil
}

// CompleteEditionRequest records the edition a claimed request opened.
func (r *Repo) CompleteEditionRequest(ctx context.Context, tx *gorm.DB, requestID uint, editionID uint) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE game_edition_requests SET edition_id = ? WHERE id = ? AND edition_id IS NULL
	`, editionID, requestID)
	if result.Error != nil {
		return fmt.Errorf("completing edition request: %s", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("edition request %d not found or already completed", requestID)
	}
	return nil
}

// FindEditionByID returns editionID's edition, or nil.
func (r *Repo) FindEditionByID(ctx context.Context, tx *gorm.DB, editionID uint) (*Edition, error) {
	var found []Edition
	if err := tx.WithContext(ctx).Raw(`SELECT id, uuid FROM game_editions WHERE id = ?`, editionID).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding edition: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// OpenEdition inserts an edition of gameID in status, with the status
// change that opened it, and returns its id.
func (r *Repo) OpenEdition(ctx context.Context, tx *gorm.DB, gameID uint, editionUUID string, status string, at time.Time) (uint, error) {
	row := editionRow{UUID: editionUUID, GameID: gameID, Status: status, CreatedAt: at}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("opening game edition: %s", err)
	}
	change := editionStatusChangeRow{EditionID: row.ID, ToStatus: status, ChangedAt: at}
	if err := tx.WithContext(ctx).Create(&change).Error; err != nil {
		return 0, fmt.Errorf("recording game edition status: %s", err)
	}
	return row.ID, nil
}
