package migrations

import (
	_ "embed"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"

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
	assetType gameversion.AssetType
	mimeType  string
	content   []byte
}

// seedSpaceDriftAssets are the assets of seedSpaceDriftAssetsVersionUUID.
var seedSpaceDriftAssets = []seedAsset{
	{key: "ship", assetType: gameversion.AssetImage, mimeType: "image/png", content: seedSpaceDriftShip},
	{key: "thrust", assetType: gameversion.AssetAudio, mimeType: "audio/wav", content: seedSpaceDriftThrust},
}

// seed20261026000000SpaceDriftAssets adds that version, asked by the human so
// the web frontend can load a game's assets end to end.
func seed20261026000000SpaceDriftAssets() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261026000000_space_drift_assets",
		Migrate: func(tx *gorm.DB) error {
			game, err := seedGameID(tx, seedSpaceDriftGameUUID)
			if err != nil {
				return err
			}
			version, err := seedCreateVersion(tx, seedExecutableVersion(seedSpaceDriftAssetsVersionUUID, game, seedSpaceDriftManifest,
				seedSpaceDriftScriptLocator, scriptcheck.ProfileHandwritten, seedSpaceDriftAssetsFrontendPackageLocator, gameversion.AspectRatio16x9))
			if err != nil {
				return err
			}
			for _, asset := range seedSpaceDriftAssets {
				locator := objectstore.LocatorFor(assetPrefix, asset.content)
				row := seedVersionAssetRow{
					GameVersionID: version, Key: asset.key, Type: string(asset.assetType), MIMEType: asset.mimeType,
					ObjectKey: &locator.ObjectKey, SHA256: locator.SHA256, Size: locator.Size,
				}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
			return seedSetCurrentVersion(tx, game, version)
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
