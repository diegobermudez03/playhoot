// Package checkplayability answers whether a Game can currently be played by a
// given user: it is published, not deleted, and public or the user's own.
package checkplayability

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
)

//go:generate mockgen -package=checkplayability -destination=service_mock_test.go . repoAPI

// repoAPI is the persistence the playability check reads.
type repoAPI interface {
	FindPlayability(ctx context.Context, gameUUID string) (*Playability, error)
}

// Playability is what deciding whether a Game is playable reads.
type Playability struct {
	OwnerUUID   string
	Visibility  string
	PublishedAt *time.Time
}

// Service is Game Management's playability-check use case.
type Service struct {
	repo repoAPI
}

// New constructs a Service reading from db.
func New(db *gorm.DB) *Service {
	return &Service{repo: &repo{db: db}}
}

// IsPlayable reports whether userUUID can currently play gameUUID: a
// published Game that is public, or private and theirs. A Game that does not
// exist, was deleted or was never published is simply not playable.
func (s *Service) IsPlayable(ctx context.Context, gameUUID string, userUUID string) (bool, error) {
	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return false, nil
	}
	found, err := s.repo.FindPlayability(ctx, parsedGame.String())
	if err != nil {
		return false, err
	}
	playable := decide(found, userUUID)
	logging.LogFields(ctx, logging.Field("game_playable", playable))
	return playable, nil
}

func decide(found *Playability, userUUID string) bool {
	if found == nil || found.PublishedAt == nil {
		return false
	}
	switch game.VisibilityType(found.Visibility) {
	case game.Public:
		return true
	case game.Private:
		owner, ownerErr := uuid.Parse(found.OwnerUUID)
		user, userErr := uuid.Parse(userUUID)
		return ownerErr == nil && userErr == nil && owner == user
	default:
		return false
	}
}

type repo struct {
	db *gorm.DB
}

// FindPlayability returns gameUUID's playability facts, or nil when there is
// no such Game or it was deleted.
func (r *repo) FindPlayability(ctx context.Context, gameUUID string) (*Playability, error) {
	var found []Playability
	if err := r.db.WithContext(ctx).Raw(`
		SELECT owner_uuid, visibility, published_at FROM games WHERE uuid = ? AND deleted_at IS NULL
	`, gameUUID).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding game playability: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}
