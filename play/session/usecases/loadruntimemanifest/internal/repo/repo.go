// Package repo is loadruntimemanifest's persistence: it reads the manifest of
// a Session's pinned game version and decides nothing itself.
package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Repo is loadruntimemanifest's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// SessionManifest is what the Runtime reads about a Session's pinned game
// version. Manifest is empty when the version row is missing, and
// InputRequirements nil when the version has no requirements row.
type SessionManifest struct {
	Manifest          []byte
	InputRequirements *InputRequirements
}

// InputRequirements are a game version's stored interactions and devices.
type InputRequirements struct {
	UsesKeyboard     bool
	UsesPointerHover bool
	UsesPointerPress bool
	Devices          string
}

// FindSessionManifest returns the manifest and input requirements of the game
// version sessionUUID is pinned to; found is false when there is no such
// Session.
func (r *Repo) FindSessionManifest(ctx context.Context, sessionUUID string) (manifest SessionManifest, found bool, err error) {
	var row struct {
		Manifest         []byte
		RequirementsID   *uint
		UsesKeyboard     *bool
		UsesPointerHover *bool
		UsesPointerPress *bool
		Devices          *string
	}
	result := r.db.WithContext(ctx).Raw(`
		SELECT v.manifest, ir.id AS requirements_id, ir.uses_keyboard, ir.uses_pointer_hover, ir.uses_pointer_press, ir.devices
		FROM play_sessions s
		LEFT JOIN play_game_versions v ON v.id = s.game_version_id
		LEFT JOIN play_game_version_input_requirements ir ON ir.game_version_id = v.id
		WHERE s.uuid = ?
	`, sessionUUID).Scan(&row)
	if result.Error != nil {
		return SessionManifest{}, false, fmt.Errorf("finding session manifest: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return SessionManifest{}, false, nil
	}
	manifest = SessionManifest{Manifest: row.Manifest}
	if row.RequirementsID != nil {
		manifest.InputRequirements = &InputRequirements{
			UsesKeyboard:     *row.UsesKeyboard,
			UsesPointerHover: *row.UsesPointerHover,
			UsesPointerPress: *row.UsesPointerPress,
			Devices:          *row.Devices,
		}
	}
	return manifest, true, nil
}
