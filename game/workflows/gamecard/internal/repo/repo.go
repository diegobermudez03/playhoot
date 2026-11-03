// Package repo is gamecard's narrow persistence layer. It reports persisted
// facts and performs the mutations the Manager requests; it decides no card
// policy itself.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repo is gamecard's persistence implementation. Its methods take the
// transaction the Manager opened; it never opens or commits one itself.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB satisfies utils.DBServicer.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}

// CardGame is a Game not deleted with its card texts and lifecycle facts.
type CardGame struct {
	ID          uint
	UUID        string
	OwnerUUID   string
	Name        string
	Description string
	Visibility  string
	PublishedAt *time.Time
}

// FindGame returns gameUUID's Game, locked until tx ends when lock is set,
// or nil when there is no such Game or it was deleted.
func (r *Repo) FindGame(ctx context.Context, tx *gorm.DB, gameUUID string, lock bool) (*CardGame, error) {
	query := `SELECT id, uuid, owner_uuid, name, description, visibility, published_at FROM games WHERE uuid = ? AND deleted_at IS NULL`
	if lock {
		query += ` FOR UPDATE`
	}
	var found []CardGame
	if err := tx.WithContext(ctx).Raw(query, gameUUID).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding game: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// LifecycleFacts is whether a Game has an edition open and a release being
// published.
type LifecycleFacts struct {
	Editing    bool
	Publishing bool
}

// FindLifecycleFacts reads gameID's lifecycle facts.
func (r *Repo) FindLifecycleFacts(ctx context.Context, tx *gorm.DB, gameID uint, editingStatus string) (LifecycleFacts, error) {
	var facts LifecycleFacts
	if err := tx.WithContext(ctx).Raw(`
		SELECT
			EXISTS (SELECT 1 FROM game_editions WHERE game_id = ? AND status = ?) AS editing,
			EXISTS (SELECT 1 FROM game_releases WHERE game_id = ? AND completed_at IS NULL AND abandoned_at IS NULL) AS publishing
	`, gameID, editingStatus, gameID).Scan(&facts).Error; err != nil {
		return LifecycleFacts{}, fmt.Errorf("finding lifecycle facts: %s", err)
	}
	return facts, nil
}

type historyRow struct {
	ID          uint
	GameID      uint
	Name        *string
	Description *string
	CreatedAt   time.Time
}

func (historyRow) TableName() string { return "game_histories" }

// UpdateTexts writes gameID's card texts and the history of those that
// changed (game_histories keeps only changed fields).
func (r *Repo) UpdateTexts(ctx context.Context, tx *gorm.DB, game CardGame, name string, description string, at time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE games SET name = ?, description = ?, updated_at = ? WHERE id = ?
	`, name, description, at, game.ID).Error; err != nil {
		return fmt.Errorf("updating card texts: %s", err)
	}
	history := historyRow{GameID: game.ID, CreatedAt: at}
	if name != game.Name {
		history.Name = &name
	}
	if description != game.Description {
		history.Description = &description
	}
	if err := tx.WithContext(ctx).Create(&history).Error; err != nil {
		return fmt.Errorf("recording card history: %s", err)
	}
	return nil
}

// Image is one image a card shows.
type Image struct {
	ID        uint
	UUID      string
	Kind      string
	SHA256    string `gorm:"column:sha256"`
	Size      int64
	MIMEType  string `gorm:"column:mime_type"`
	ObjectKey string
	CreatedAt time.Time
}

// FindActiveImages returns the images gameID's card shows, oldest first.
func (r *Repo) FindActiveImages(ctx context.Context, tx *gorm.DB, gameID uint) ([]Image, error) {
	var images []Image
	if err := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, kind, sha256, size, mime_type, object_key, created_at
		FROM game_card_images WHERE game_id = ? AND removed_at IS NULL ORDER BY id
	`, gameID).Scan(&images).Error; err != nil {
		return nil, fmt.Errorf("finding card images: %s", err)
	}
	return images, nil
}

// FindImageByID returns imageID, removed or not, or nil.
func (r *Repo) FindImageByID(ctx context.Context, tx *gorm.DB, imageID uint) (*Image, error) {
	var images []Image
	if err := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, kind, sha256, size, mime_type, object_key, created_at FROM game_card_images WHERE id = ?
	`, imageID).Scan(&images).Error; err != nil {
		return nil, fmt.Errorf("finding card image: %s", err)
	}
	if len(images) == 0 {
		return nil, nil
	}
	return &images[0], nil
}

type imageRow struct {
	ID        uint
	UUID      string
	GameID    uint
	Kind      string
	SHA256    string `gorm:"column:sha256"`
	Size      int64
	MIMEType  string `gorm:"column:mime_type"`
	ObjectKey string
	CreatedAt time.Time
}

func (imageRow) TableName() string { return "game_card_images" }

// CreateImage records an image gameID's card shows and returns its id.
func (r *Repo) CreateImage(ctx context.Context, tx *gorm.DB, gameID uint, image Image) (uint, error) {
	row := imageRow{
		UUID: image.UUID, GameID: gameID, Kind: image.Kind, SHA256: image.SHA256, Size: image.Size,
		MIMEType: image.MIMEType, ObjectKey: image.ObjectKey, CreatedAt: image.CreatedAt,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("recording card image: %s", err)
	}
	return row.ID, nil
}

// RemoveImage writes imageID's removal once; false means it was already
// removed.
func (r *Repo) RemoveImage(ctx context.Context, tx *gorm.DB, imageID uint, at time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE game_card_images SET removed_at = ? WHERE id = ? AND removed_at IS NULL
	`, at, imageID)
	if result.Error != nil {
		return false, fmt.Errorf("removing card image: %s", result.Error)
	}
	return result.RowsAffected == 1, nil
}

type objectRow struct {
	ID           uint
	SHA256       string `gorm:"column:sha256"`
	Size         int64
	MIMEType     string `gorm:"column:mime_type"`
	ObjectKey    string
	CreatedAt    time.Time
	RegisteredAt time.Time
}

func (objectRow) TableName() string { return "game_card_objects" }

// RegisterObject records card image content about to be stored, once per
// hash; registering it again marks it as being stored now.
func (r *Repo) RegisterObject(ctx context.Context, tx *gorm.DB, sha256 string, size int64, mimeType string, objectKey string, at time.Time) error {
	row := objectRow{SHA256: sha256, Size: size, MIMEType: mimeType, ObjectKey: objectKey, CreatedAt: at, RegisteredAt: at}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("registering card object: %s", err)
	}
	if err := tx.WithContext(ctx).Exec(`
		UPDATE game_card_objects SET registered_at = ?, deleted_at = NULL WHERE sha256 = ?
	`, at, sha256).Error; err != nil {
		return fmt.Errorf("registering card object again: %s", err)
	}
	return nil
}

// ImageRequest is a claimed card image upload request; ImageID is nil until
// the upload that claimed it completes it.
type ImageRequest struct {
	ID      uint
	Kind    string
	SHA256  string `gorm:"column:sha256"`
	ImageID *uint
}

type imageRequestRow struct {
	ID             uint
	GameID         uint
	IdempotencyKey string
	Kind           string
	SHA256         string `gorm:"column:sha256"`
	CreatedAt      time.Time
}

func (imageRequestRow) TableName() string { return "game_card_image_requests" }

// FindImageRequest returns gameID's upload request holding idempotencyKey,
// or nil.
func (r *Repo) FindImageRequest(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string) (*ImageRequest, error) {
	var found []ImageRequest
	if err := tx.WithContext(ctx).Raw(`
		SELECT id, kind, sha256, image_id FROM game_card_image_requests WHERE game_id = ? AND idempotency_key = ?
	`, gameID, idempotencyKey).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding card image request: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// ClaimImageRequest records an upload request and returns its id.
func (r *Repo) ClaimImageRequest(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string, kind string, sha256 string, at time.Time) (uint, error) {
	row := imageRequestRow{GameID: gameID, IdempotencyKey: idempotencyKey, Kind: kind, SHA256: sha256, CreatedAt: at}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("claiming card image request: %s", err)
	}
	if row.ID == 0 {
		return 0, fmt.Errorf("card image request was claimed meanwhile")
	}
	return row.ID, nil
}

// CompleteImageRequest records the image an upload request bound.
func (r *Repo) CompleteImageRequest(ctx context.Context, tx *gorm.DB, requestID uint, imageID uint) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE game_card_image_requests SET image_id = ? WHERE id = ? AND image_id IS NULL
	`, imageID, requestID)
	if result.Error != nil {
		return fmt.Errorf("completing card image request: %s", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("card image request %d not found or already completed", requestID)
	}
	return nil
}
