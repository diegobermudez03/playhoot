package migrations

import (
	_ "embed"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
)

// seedSpaceDriftAssetsVersionUUID is Space Drift's third EXECUTABLE version:
// the same manifest and backend script as the second, a frontend package that
// declares and loads assets, and those assets (seeddata/README.md). It becomes the Game's current version, so new
// Sessions use it; Sessions already pinned to the others keep theirs.
const seedSpaceDriftAssetsVersionUUID = "33333333-3333-4333-8333-0000000000c3"

// assetPrefix is the object storage prefix of game version assets, which are
// content-addressed below it.
const assetPrefix = "game-versions/assets"

// Space Drift's seed assets: placeholder media for testing asset delivery,
// and the frontend package that loads them.
var (
	//go:embed seeddata/space_drift_assets_frontend.html
	seedSpaceDriftAssetsFrontendPackage []byte
	//go:embed seeddata/space_drift_ship.png
	seedSpaceDriftShip []byte
	//go:embed seeddata/space_drift_thrust.wav
	seedSpaceDriftThrust []byte
)

var seedSpaceDriftAssetsFrontendPackageLocator = objectstore.LocatorFor(frontendPackagePrefix, seedSpaceDriftAssetsFrontendPackage)

// seedAsset is one asset of a seed game version.
type seedAsset struct {
	key       string
	assetType string
	mimeType  string
	content   []byte
}

// seedSpaceDriftAssets are the assets of seedSpaceDriftAssetsVersionUUID.
var seedSpaceDriftAssets = []seedAsset{
	{key: "ship", assetType: "image", mimeType: "image/png", content: seedSpaceDriftShip},
	{key: "thrust", assetType: "audio", mimeType: "audio/wav", content: seedSpaceDriftThrust},
}

// seed20261026000000SpaceDriftAssets adds that version, asked by the human so
// the web frontend can load a game's assets end to end.
func seed20261026000000SpaceDriftAssets() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261026000000_space_drift_assets",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`INSERT INTO play_game_versions (uuid, game_id, status, manifest,
					script_object_key, script_sha256, script_size,
					frontend_object_key, frontend_sha256, frontend_size, aspect_ratio, executable_at)
				SELECT ?, id, 'EXECUTABLE', ?, ?, ?, ?, ?, ?, ?, '16:9', CURRENT_TIMESTAMP FROM play_games WHERE game_uuid = ?`,
				seedSpaceDriftAssetsVersionUUID, seedSpaceDriftManifest,
				seedSpaceDriftScriptLocator.ObjectKey, seedSpaceDriftScriptLocator.SHA256, seedSpaceDriftScriptLocator.Size,
				seedSpaceDriftAssetsFrontendPackageLocator.ObjectKey, seedSpaceDriftAssetsFrontendPackageLocator.SHA256, seedSpaceDriftAssetsFrontendPackageLocator.Size,
				seedSpaceDriftGameUUID,
			).Error; err != nil {
				return err
			}
			for _, asset := range seedSpaceDriftAssets {
				locator := objectstore.LocatorFor(assetPrefix, asset.content)
				if err := tx.Exec(`INSERT INTO play_game_version_assets (game_version_id, key, type, mime_type, object_key, sha256, size)
					SELECT id, ?, ?, ?, ?, ?, ? FROM play_game_versions WHERE uuid = ?`,
					asset.key, asset.assetType, asset.mimeType, locator.ObjectKey, locator.SHA256, locator.Size, seedSpaceDriftAssetsVersionUUID,
				).Error; err != nil {
					return err
				}
			}
			return tx.Exec(`UPDATE play_games SET current_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
				WHERE game_uuid = ?`, seedSpaceDriftAssetsVersionUUID, seedSpaceDriftGameUUID).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				UPDATE play_games SET current_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
				WHERE game_uuid = ?
			`, seedSpaceDriftFrontendVersionUUID, seedSpaceDriftGameUUID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DELETE FROM play_game_version_assets
				WHERE game_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)`, seedSpaceDriftAssetsVersionUUID).Error; err != nil {
				return err
			}
			return tx.Exec(`DELETE FROM play_game_versions WHERE uuid = ?`, seedSpaceDriftAssetsVersionUUID).Error
		},
	}
}
