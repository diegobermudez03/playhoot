package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/session/internal/objectstore"
	"gorm.io/gorm"
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

// GameVersionArtifact is the narrow view of a pinned
// session_game_version_artifacts row this package's steps need: where the
// backend script the Executor evaluates is stored (BackendScript - the bytes
// themselves live in object storage and are loaded through a verified
// loader, never held in the database), the structural participant-count
// range Playhoot itself enforces before any script runs (ParticipantMax nil
// means unlimited) - session/docs/GAME_VERSION_ARTIFACT_MODEL.md's own
// "Participant Constraints" section - and ProjectionVisibility, the
// author's own declared privacy schema a viewer-scoped filtering step
// parses before computing that viewer's own ClientState (nil means no
// schema declared - treated as maximally restrictive, not an error).
// Every other artifact field is not this package's concern.
type GameVersionArtifact struct {
	BackendScript        objectstore.Locator
	ParticipantMin       int
	ParticipantMax       *int
	ProjectionVisibility json.RawMessage
}

// gameVersionArtifactRow is this query's raw column shape.
type gameVersionArtifactRow struct {
	BackendScriptKey     string          `gorm:"column:backend_script_key"`
	BackendScriptSHA256  string          `gorm:"column:backend_script_sha256"`
	BackendScriptSize    int64           `gorm:"column:backend_script_size"`
	ParticipantMin       int             `gorm:"column:participant_min"`
	ParticipantMax       *int            `gorm:"column:participant_max"`
	ProjectionVisibility json.RawMessage `gorm:"column:projection_visibility"`
}

const gameVersionArtifactColumns = `backend_script_key, backend_script_sha256, backend_script_size, participant_min, participant_max, projection_visibility`

func (row gameVersionArtifactRow) toArtifact() *GameVersionArtifact {
	return &GameVersionArtifact{
		BackendScript:        objectstore.Locator{ObjectKey: row.BackendScriptKey, SHA256: row.BackendScriptSHA256, Size: row.BackendScriptSize},
		ParticipantMin:       row.ParticipantMin,
		ParticipantMax:       row.ParticipantMax,
		ProjectionVisibility: row.ProjectionVisibility,
	}
}

// ResolveGameVersionArtifact is an unlocked, pre-transaction lookup by
// definitionUUID - Join's own read, mirroring ResolveCurrentGameDefinitionUUID's
// unlocked style: capacity must stay governed by the exact version pinned at
// Create, read before the mutation transaction/row lock opens. A pure read
// like GetClientState also uses this unlocked read - it never mutates
// state, so no transaction/row lock is needed the way a RUNNING-phase
// mutation's own GetGameVersionArtifact requires.
func (r *Repo) ResolveGameVersionArtifact(ctx context.Context, definitionUUID string) (*GameVersionArtifact, error) {
	var row gameVersionArtifactRow
	result := r.db.WithContext(ctx).Raw(`
		SELECT `+gameVersionArtifactColumns+`
		FROM session_game_version_artifacts
		WHERE definition_uuid = ?
	`, definitionUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving game version artifact: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return row.toArtifact(), nil
}

// GetGameVersionArtifact loads definitionUUID's pinned artifact within an
// already-open transaction - every RUNNING-phase step's own read, since each
// already holds tx by the time it needs this.
func (r *Repo) GetGameVersionArtifact(ctx context.Context, tx *gorm.DB, definitionUUID string) (*GameVersionArtifact, error) {
	var row gameVersionArtifactRow
	result := tx.WithContext(ctx).Raw(`
		SELECT `+gameVersionArtifactColumns+`
		FROM session_game_version_artifacts
		WHERE definition_uuid = ?
	`, definitionUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("getting game version artifact: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return row.toArtifact(), nil
}

// ContentObject is one servable stored object of a pinned version: where it
// lives and what a reader needs to verify and interpret what it fetches.
type ContentObject struct {
	Locator     objectstore.Locator
	ContentType string
}

// frontendScriptContentType is the content type recorded for every frontend
// script: its language/format is not decided, so it is served as opaque
// JavaScript source text.
const frontendScriptContentType = "text/javascript"

// ResolveFrontendScriptObject is an unlocked read of definitionUUID's
// frontend script locator. Returns nil, nil if no such version exists.
func (r *Repo) ResolveFrontendScriptObject(ctx context.Context, definitionUUID string) (*ContentObject, error) {
	var row struct {
		Key    string `gorm:"column:frontend_script_key"`
		SHA256 string `gorm:"column:frontend_script_sha256"`
		Size   int64  `gorm:"column:frontend_script_size"`
	}
	result := r.db.WithContext(ctx).Raw(`
		SELECT frontend_script_key, frontend_script_sha256, frontend_script_size
		FROM session_game_version_artifacts
		WHERE definition_uuid = ?
	`, definitionUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving frontend script object: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &ContentObject{
		Locator:     objectstore.Locator{ObjectKey: row.Key, SHA256: row.SHA256, Size: row.Size},
		ContentType: frontendScriptContentType,
	}, nil
}

// ResolveAssetObject is an unlocked read of the asset declared under
// (definitionUUID, key). Returns nil, nil if that version declares no asset
// with that key.
func (r *Repo) ResolveAssetObject(ctx context.Context, definitionUUID, key string) (*ContentObject, error) {
	var row struct {
		ObjectKey   string `gorm:"column:object_key"`
		SHA256      string `gorm:"column:sha256"`
		Size        int64  `gorm:"column:size"`
		ContentType string `gorm:"column:content_type"`
	}
	result := r.db.WithContext(ctx).Raw(`
		SELECT object_key, sha256, size, content_type
		FROM session_game_version_assets
		WHERE definition_uuid = ? AND key = ?
	`, definitionUUID, key).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving asset object: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &ContentObject{
		Locator:     objectstore.Locator{ObjectKey: row.ObjectKey, SHA256: row.SHA256, Size: row.Size},
		ContentType: row.ContentType,
	}, nil
}
