package repo

import (
	"context"
	"fmt"
)

// ResolveCurrentGameDefinitionUUID resolves gameUUID's currently pinnable
// version entirely from Session Runtime's own session_games table - never
// from Game Management. Returns nil, nil if no session_games row exists for
// gameUUID, or its current_definition_uuid is not yet set (no version has
// ever been published for this Game). An unlocked, pre-transaction read,
// mirroring ResolveSessionForJoinCode: the resolved value is only used to
// pin a brand-new Session, never compared against existing state, so no
// later re-validation under lock is needed.
func (r *Repo) ResolveCurrentGameDefinitionUUID(ctx context.Context, gameUUID string) (*string, error) {
	var resolution struct {
		CurrentDefinitionUUID *string `gorm:"column:current_definition_uuid"`
	}
	result := r.db.WithContext(ctx).Raw(`
		SELECT current_definition_uuid
		FROM session_games
		WHERE game_uuid = ?
	`, gameUUID).Scan(&resolution)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving current game definition: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return resolution.CurrentDefinitionUUID, nil
}
