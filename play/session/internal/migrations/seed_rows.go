package migrations

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputrequirements"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
)

// Rows the seed migrations insert, mapped explicitly so no insert depends on
// positional arguments.
type seedGameRow struct {
	ID       uint
	GameUUID string
}

func (seedGameRow) TableName() string { return "play_games" }

type seedVersionRow struct {
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

func (seedVersionRow) TableName() string { return "play_game_versions" }

type seedVersionAssetRow struct {
	ID            uint
	GameVersionID uint
	Key           string
	Type          string
	MIMEType      string `gorm:"column:mime_type"`
	ObjectKey     *string
	SHA256        string `gorm:"column:sha256"`
	Size          int64
}

func (seedVersionAssetRow) TableName() string { return "play_game_version_assets" }

type seedInputRequirementsRow struct {
	ID               uint
	GameVersionID    uint
	UsesKeyboard     bool
	UsesPointerHover bool
	UsesPointerPress bool
	Devices          string
}

func (seedInputRequirementsRow) TableName() string {
	return "play_game_version_input_requirements"
}

// seedCreateGame inserts a Game and returns its id.
func seedCreateGame(tx *gorm.DB, gameUUID string) (uint, error) {
	game := seedGameRow{GameUUID: gameUUID}
	if err := tx.Create(&game).Error; err != nil {
		return 0, err
	}
	return game.ID, nil
}

// seedGameID reads the id of a Game an earlier seed inserted.
func seedGameID(tx *gorm.DB, gameUUID string) (uint, error) {
	var ids []uint
	if err := tx.Raw(`SELECT id FROM play_games WHERE game_uuid = ?`, gameUUID).Scan(&ids).Error; err != nil {
		return 0, err
	}
	if len(ids) != 1 {
		return 0, fmt.Errorf("seed game %s not found", gameUUID)
	}
	return ids[0], nil
}

// seedVersionIDs reads the ids of versions earlier seeds inserted, in the
// order of versionUUIDs.
func seedVersionIDs(tx *gorm.DB, versionUUIDs []string) ([]uint, error) {
	var found []struct {
		ID   uint
		UUID string
	}
	if err := tx.Raw(`SELECT id, uuid FROM play_game_versions WHERE uuid IN ?`, versionUUIDs).Scan(&found).Error; err != nil {
		return nil, err
	}
	byUUID := make(map[string]uint, len(found))
	for _, version := range found {
		byUUID[version.UUID] = version.ID
	}
	ids := make([]uint, len(versionUUIDs))
	for i, versionUUID := range versionUUIDs {
		id, ok := byUUID[versionUUID]
		if !ok {
			return nil, fmt.Errorf("seed version %s not found", versionUUID)
		}
		ids[i] = id
	}
	return ids, nil
}

// seedExecutableVersion is an EXECUTABLE version of gameID with its script,
// frontend package and aspect ratio.
func seedExecutableVersion(versionUUID string, gameID uint, manifest string, script objectstore.Locator, profile scriptcheck.Profile, frontend objectstore.Locator, ratio gameversion.AspectRatio) seedVersionRow {
	executableAt := time.Now()
	ratioText := string(ratio)
	return seedVersionRow{
		UUID:              versionUUID,
		GameID:            gameID,
		Status:            string(gameversion.Executable),
		Manifest:          &manifest,
		ScriptObjectKey:   &script.ObjectKey,
		ScriptSHA256:      &script.SHA256,
		ScriptSize:        &script.Size,
		ScriptProfile:     string(profile),
		FrontendObjectKey: &frontend.ObjectKey,
		FrontendSHA256:    &frontend.SHA256,
		FrontendSize:      &frontend.Size,
		AspectRatio:       &ratioText,
		ExecutableAt:      &executableAt,
	}
}

// seedDraftVersion is a DRAFT version of gameID holding only its manifest.
func seedDraftVersion(versionUUID string, gameID uint, manifest string) seedVersionRow {
	return seedVersionRow{
		UUID:          versionUUID,
		GameID:        gameID,
		Status:        string(gameversion.Draft),
		Manifest:      &manifest,
		ScriptProfile: string(scriptcheck.ProfileHandwritten),
	}
}

// seedCreateVersion inserts version and returns its id.
func seedCreateVersion(tx *gorm.DB, version seedVersionRow) (uint, error) {
	if err := tx.Create(&version).Error; err != nil {
		return 0, err
	}
	return version.ID, nil
}

// seedSetCurrentVersion points gameID at versionID.
func seedSetCurrentVersion(tx *gorm.DB, gameID uint, versionID uint) error {
	return tx.Exec(`UPDATE play_games SET current_version_id = ? WHERE id = ?`, versionID, gameID).Error
}

// seedCreateInputRequirements records requirements for each version id.
func seedCreateInputRequirements(tx *gorm.DB, versionIDs []uint, requirements inputrequirements.Requirements) error {
	if err := requirements.Validate(); err != nil {
		return err
	}
	for _, versionID := range versionIDs {
		row := seedInputRequirementsRow{
			GameVersionID:    versionID,
			UsesKeyboard:     requirements.UsesKeyboard,
			UsesPointerHover: requirements.UsesPointerHover,
			UsesPointerPress: requirements.UsesPointerPress,
			Devices:          string(requirements.Devices),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// seedPressOnly is what the seed games ask of a device: pressing or dragging
// only, so a desktop or a touch device can play them.
func seedPressOnly() inputrequirements.Requirements {
	return inputrequirements.Requirements{
		UsesPointerPress: true,
		Devices:          inputrequirements.DevicesFor(false, false, true),
	}
}
