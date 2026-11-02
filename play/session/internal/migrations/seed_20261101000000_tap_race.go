package migrations

import (
	_ "embed"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputrequirements"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
)

// Tap Race test-data identifiers, so a person testing by hand can create its
// Sessions.
const (
	// seedTapRaceGameUUID is a playable Game whose EXECUTABLE version is the
	// first game built with the Playhoot SDK: one to eight players tap, the
	// first to 20 taps wins, or whoever leads after 30 seconds.
	seedTapRaceGameUUID    = "55555555-5555-4555-8555-555555555555"
	seedTapRaceVersionUUID = "55555555-5555-4555-8555-0000000000e1"

	// seedTapRaceManifest is the manifest the SDK build derived from the
	// game's definition (seeddata/README.md).
	seedTapRaceManifest = `{"schema_version":1,"players":{"min":1,"max":8},"inputs":{},"commands":{"tap":{"payload":{}}},"entrypoints":["update"]}`
)

// seedTapRaceScript and seedTapRaceFrontendPackage are Tap Race's backend
// script (profile sdk-v1) and frontend package, built by the frontend
// repository's game tools (seeddata/README.md).
var (
	//go:embed seeddata/tap_race_server.js
	seedTapRaceScript []byte
	//go:embed seeddata/tap_race_frontend.html
	seedTapRaceFrontendPackage []byte

	seedTapRaceScriptLocator          = objectstore.LocatorFor(backendScriptPrefix, seedTapRaceScript)
	seedTapRaceFrontendPackageLocator = objectstore.LocatorFor(frontendPackagePrefix, seedTapRaceFrontendPackage)
)

// Rows the Tap Race seed inserts, mapped explicitly.
type seedTapRaceGameRow struct {
	ID       uint
	GameUUID string
}

func (seedTapRaceGameRow) TableName() string { return "play_games" }

type seedTapRaceVersionRow struct {
	ID                uint
	UUID              string
	GameID            uint
	Status            string
	Manifest          *string `gorm:"type:jsonb"`
	ScriptObjectKey   *string
	ScriptSHA256      *string `gorm:"column:script_sha256"`
	ScriptSize        *int64
	ScriptProfile     string
	FrontendObjectKey *string
	FrontendSHA256    *string `gorm:"column:frontend_sha256"`
	FrontendSize      *int64
	AspectRatio       *string
	ExecutableAt      *time.Time
}

func (seedTapRaceVersionRow) TableName() string { return "play_game_versions" }

type seedTapRaceInputRequirementsRow struct {
	ID               uint
	GameVersionID    uint
	UsesKeyboard     bool
	UsesPointerHover bool
	UsesPointerPress bool
	Devices          string
}

func (seedTapRaceInputRequirementsRow) TableName() string {
	return "play_game_version_input_requirements"
}

// seed20261101000000TapRace adds the Tap Race Game and its EXECUTABLE
// version, to play an SDK-built game end to end (game creation project,
// CHECKLIST B).
func seed20261101000000TapRace() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261101000000_tap_race",
		Migrate: func(tx *gorm.DB) error {
			pressOnly := inputrequirements.Requirements{
				UsesPointerPress: true,
				Devices:          inputrequirements.DevicesFor(false, false, true),
			}
			if err := pressOnly.Validate(); err != nil {
				return err
			}
			game := seedTapRaceGameRow{GameUUID: seedTapRaceGameUUID}
			if err := tx.Create(&game).Error; err != nil {
				return err
			}
			manifest, profile, ratio := seedTapRaceManifest, string(scriptcheck.ProfileSDKV1), string(gameversion.AspectRatio16x9)
			now := time.Now()
			version := seedTapRaceVersionRow{
				UUID:              seedTapRaceVersionUUID,
				GameID:            game.ID,
				Status:            string(gameversion.Executable),
				Manifest:          &manifest,
				ScriptObjectKey:   &seedTapRaceScriptLocator.ObjectKey,
				ScriptSHA256:      &seedTapRaceScriptLocator.SHA256,
				ScriptSize:        &seedTapRaceScriptLocator.Size,
				ScriptProfile:     profile,
				FrontendObjectKey: &seedTapRaceFrontendPackageLocator.ObjectKey,
				FrontendSHA256:    &seedTapRaceFrontendPackageLocator.SHA256,
				FrontendSize:      &seedTapRaceFrontendPackageLocator.Size,
				AspectRatio:       &ratio,
				ExecutableAt:      &now,
			}
			if err := tx.Create(&version).Error; err != nil {
				return err
			}
			requirements := seedTapRaceInputRequirementsRow{
				GameVersionID:    version.ID,
				UsesKeyboard:     pressOnly.UsesKeyboard,
				UsesPointerHover: pressOnly.UsesPointerHover,
				UsesPointerPress: pressOnly.UsesPointerPress,
				Devices:          string(pressOnly.Devices),
			}
			if err := tx.Create(&requirements).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE play_games SET current_version_id = ? WHERE id = ?`, version.ID, game.ID).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DELETE FROM play_game_version_input_requirements
				WHERE game_version_id IN (SELECT v.id FROM play_game_versions v JOIN play_games g ON g.id = v.game_id WHERE g.game_uuid = ?)
			`, seedTapRaceGameUUID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				DELETE FROM play_game_versions
				WHERE game_id IN (SELECT id FROM play_games WHERE game_uuid = ?)
			`, seedTapRaceGameUUID).Error; err != nil {
				return err
			}
			return tx.Exec(`DELETE FROM play_games WHERE game_uuid = ?`, seedTapRaceGameUUID).Error
		},
	}
}
