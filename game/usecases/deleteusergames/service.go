// Package deleteusergames removes the games of a person who no longer
// exists. Sessions already running from them are not interrupted, and the
// history of everyone who played them stays as it is.
package deleteusergames

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

// batch is how many games one statement deletes.
const batch = 500

var deletedTotal = metrics.NewCounter(
	"game_owner_games_deleted_total",
	"Games deleted because their owner's identity was deleted.",
)

// Service deletes the games of a deleted owner.
type Service struct {
	db  *gorm.DB
	now func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// DeleteUserGames deletes every game ownerUUID still owns, answering how
// many it deleted. Games already deleted are left alone, so running it
// again deletes nothing new.
func (s *Service) DeleteUserGames(ctx context.Context, ownerUUID string) (int64, error) {
	defer logging.Step(ctx, "game.delete_user_games").Close()

	now := s.now().UTC()
	var total int64
	for {
		result := s.db.WithContext(ctx).Exec(`
			UPDATE games SET deleted_at = ?, updated_at = ?
			WHERE id IN (
				SELECT id FROM games WHERE owner_uuid = ? AND deleted_at IS NULL ORDER BY id LIMIT ?
			)`,
			now, now, ownerUUID, batch,
		)
		if result.Error != nil {
			return total, fmt.Errorf("deleting the games of a deleted owner: %s", result.Error)
		}
		total += result.RowsAffected
		if result.RowsAffected < batch {
			break
		}
	}
	if total > 0 {
		deletedTotal.Add(ctx, float64(total))
	}
	logging.LogFields(ctx, logging.Field("owner_games_deleted", total))
	return total, nil
}
