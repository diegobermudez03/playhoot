package repo

import (
	"context"
	"fmt"

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
// session_game_version_artifacts row this package's steps need: the backend
// script the Executor evaluates, and the structural participant-count range
// Playhoot itself enforces before any script runs (ParticipantMax nil means
// unlimited) - session/docs/GAME_VERSION_ARTIFACT_MODEL.md's own
// "Participant Constraints" section. Every other artifact field
// (FrontendScript, GameContract, Assets, PlatformContractVersion) is not
// this package's concern.
type GameVersionArtifact struct {
	BackendScript  string
	ParticipantMin int
	ParticipantMax *int
}

// gameVersionArtifactRow is this query's raw column shape.
type gameVersionArtifactRow struct {
	BackendScript  string `gorm:"column:backend_script"`
	ParticipantMin int    `gorm:"column:participant_min"`
	ParticipantMax *int   `gorm:"column:participant_max"`
}

func (row gameVersionArtifactRow) toArtifact() *GameVersionArtifact {
	return &GameVersionArtifact{
		BackendScript:  row.BackendScript,
		ParticipantMin: row.ParticipantMin,
		ParticipantMax: row.ParticipantMax,
	}
}

// ResolveGameVersionArtifact is an unlocked, pre-transaction lookup by
// definitionUUID - Join's own read, mirroring ResolveCurrentGameDefinitionUUID's
// unlocked style: capacity must stay governed by the exact version pinned at
// Create, read before the mutation transaction/row lock opens.
func (r *Repo) ResolveGameVersionArtifact(ctx context.Context, definitionUUID string) (*GameVersionArtifact, error) {
	var row gameVersionArtifactRow
	result := r.db.WithContext(ctx).Raw(`
		SELECT backend_script, participant_min, participant_max
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
		SELECT backend_script, participant_min, participant_max
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
