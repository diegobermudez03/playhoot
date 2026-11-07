package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/diegobermudez03/playhoot/game"
)

// CreateGameRequest is a claimed Game-creation idempotency request. GameID is
// nil until the creation that claimed it completes it.
type CreateGameRequest struct {
	ID               uint
	GameID           *uint
	Name             *string
	TemplateGameUUID *string
}

// NewGame is a Game about to be created, unpublished, with its card empty.
type NewGame struct {
	UUID       string
	OwnerUUID  string
	Name       string
	Visibility string
	CreatedAt  time.Time
}

// CreatedGame is a Game with the first edition it was created with.
type CreatedGame struct {
	GameUUID           string
	EditionVersionUUID string
	CreatedAt          time.Time
}

type createGameRequest struct {
	ID               uint
	OwnerUUID        string
	IdempotencyKey   string
	Name             *string
	TemplateGameUUID *string
}

func (createGameRequest) TableName() string { return "game_create_requests" }

type gameRow struct {
	ID          uint
	UUID        string
	Name        string
	Description string
	OwnerUUID   string
	Visibility  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (gameRow) TableName() string { return "games" }

type gameHistoryRow struct {
	ID          uint
	GameID      uint
	Name        *string
	Description *string
	Visibility  *string
	IsPublished *bool
	CreatedAt   time.Time
}

func (gameHistoryRow) TableName() string { return "game_histories" }

type editionRow struct {
	ID                  uint
	UUID                string
	GameID              uint
	Status              string
	TemplateGameUUID    *string
	TemplateVersionUUID *string
	CreatedAt           time.Time
}

func (editionRow) TableName() string { return "game_editions" }

type editionStatusChangeRow struct {
	ID         uint
	EditionID  uint
	FromStatus *string
	ToStatus   string
	ChangedAt  time.Time
}

func (editionStatusChangeRow) TableName() string { return "game_edition_status_changes" }

// ClaimCreateGameRequest claims (ownerUUID, idempotencyKey), or returns the
// request that already holds it. A concurrent claim of the same identity waits
// for the first transaction and then sees its request.
func (r *Repo) ClaimCreateGameRequest(ctx context.Context, tx *gorm.DB, ownerUUID string, idempotencyKey string, name string, templateGameUUID string) (uint, *CreateGameRequest, error) {
	claim := createGameRequest{OwnerUUID: ownerUUID, IdempotencyKey: idempotencyKey, Name: &name}
	if templateGameUUID != "" {
		claim.TemplateGameUUID = &templateGameUUID
	}
	created := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&claim)
	if created.Error != nil {
		return 0, nil, fmt.Errorf("claiming create game request: %s", created.Error)
	}
	if created.RowsAffected == 1 {
		return claim.ID, nil, nil
	}

	var existing CreateGameRequest
	read := tx.WithContext(ctx).Raw(`
		SELECT id, game_id, name, template_game_uuid::text AS template_game_uuid
		FROM game_create_requests
		WHERE owner_uuid = ? AND idempotency_key = ?
	`, ownerUUID, idempotencyKey).Scan(&existing)
	if read.Error != nil {
		return 0, nil, fmt.Errorf("reading existing create game request: %s", read.Error)
	}
	if read.RowsAffected != 1 {
		return 0, nil, fmt.Errorf("create game request was neither claimed nor found")
	}
	return 0, &existing, nil
}

// CompleteCreateGameRequest records the Game a claimed request created.
func (r *Repo) CompleteCreateGameRequest(ctx context.Context, tx *gorm.DB, requestID uint, gameID uint) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE game_create_requests SET game_id = ? WHERE id = ? AND game_id IS NULL
	`, gameID, requestID)
	if result.Error != nil {
		return fmt.Errorf("completing create game request: %s", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("create game request %d not found or already completed", requestID)
	}
	return nil
}

// CreateGame inserts game with an empty card, and the history record that
// captures every field of a new Game (game_histories), returning its id.
func (r *Repo) CreateGame(ctx context.Context, tx *gorm.DB, game NewGame) (uint, error) {
	row := gameRow{
		UUID:       game.UUID,
		Name:       game.Name,
		OwnerUUID:  game.OwnerUUID,
		Visibility: game.Visibility,
		CreatedAt:  game.CreatedAt,
		UpdatedAt:  game.CreatedAt,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating game: %s", err)
	}
	empty, unpublished := "", false
	history := gameHistoryRow{
		GameID:      row.ID,
		Name:        &row.Name,
		Description: &empty,
		Visibility:  &row.Visibility,
		IsPublished: &unpublished,
		CreatedAt:   game.CreatedAt,
	}
	if err := tx.WithContext(ctx).Create(&history).Error; err != nil {
		return 0, fmt.Errorf("recording game creation history: %s", err)
	}
	return row.ID, nil
}

// CreateEdition inserts an edition of gameID in status, with the status
// change that opened it.
func (r *Repo) CreateEdition(ctx context.Context, tx *gorm.DB, gameID uint, editionUUID string, status string, template *game.TemplateSource, createdAt time.Time) error {
	row := editionRow{UUID: editionUUID, GameID: gameID, Status: status, CreatedAt: createdAt}
	if template != nil {
		row.TemplateGameUUID, row.TemplateVersionUUID = &template.GameUUID, &template.VersionUUID
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating game edition: %s", err)
	}
	change := editionStatusChangeRow{EditionID: row.ID, ToStatus: status, ChangedAt: createdAt}
	if err := tx.WithContext(ctx).Create(&change).Error; err != nil {
		return fmt.Errorf("recording game edition status: %s", err)
	}
	return nil
}

// FindCreatedGame returns gameID with the first edition it was created with,
// or nil when there is no such Game.
func (r *Repo) FindCreatedGame(ctx context.Context, tx *gorm.DB, gameID uint) (*CreatedGame, error) {
	var created CreatedGame
	result := tx.WithContext(ctx).Raw(`
		SELECT g.uuid AS game_uuid, e.uuid AS edition_version_uuid, g.created_at
		FROM games g
		JOIN game_editions e ON e.game_id = g.id
		WHERE g.id = ?
		ORDER BY e.id
		LIMIT 1
	`, gameID).Scan(&created)
	if result.Error != nil {
		return nil, fmt.Errorf("finding created game: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	created.CreatedAt = created.CreatedAt.UTC()
	return &created, nil
}
