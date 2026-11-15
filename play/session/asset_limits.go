package session

import "github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"

// MaxDraftAssetBytesOf is the most one asset of assetType (image, audio,
// video) may weigh, or 0 for a type Playhoot does not know.
func MaxDraftAssetBytesOf(assetType string) int64 {
	return gameversion.MaxAssetBytes(gameversion.AssetType(assetType))
}
