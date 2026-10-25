package gameversion

import (
	"fmt"
	"regexp"
)

// AssetType is what kind of media a game version's asset is. A sprite or a
// sprite sheet is an image.
type AssetType string

const (
	AssetImage AssetType = "image"
	AssetAudio AssetType = "audio"
	AssetVideo AssetType = "video"
)

// MaxAssetsBytes is the most a version's assets may weigh together.
const MaxAssetsBytes = 100 << 20

// assetRule is what Playhoot accepts for one asset type.
type assetRule struct {
	mimeTypes map[string]bool
	maxBytes  int64
}

// assetRules is the closed catalog of asset types, their MIME types and the
// most one asset of each may weigh.
var assetRules = map[AssetType]assetRule{
	AssetImage: {mimeTypes: map[string]bool{"image/png": true, "image/webp": true, "image/jpeg": true, "image/svg+xml": true}, maxBytes: 5 << 20},
	AssetAudio: {mimeTypes: map[string]bool{"audio/mpeg": true, "audio/ogg": true, "audio/wav": true}, maxBytes: 10 << 20},
	AssetVideo: {mimeTypes: map[string]bool{"video/mp4": true, "video/webm": true}, maxBytes: 50 << 20},
}

// assetKeyPattern keeps a key usable as a key of a JavaScript object literal
// in the game's frontend.
var assetKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// assetSHA256Pattern is a SHA-256 digest as stored: lowercase hex.
var assetSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Asset is a media file a game version's frontend uses, by logical key.
type Asset struct {
	Key      string
	Type     AssetType
	MIMEType string
	SHA256   string
	Size     int64
}

// ValidateAsset reports why asset breaks Playhoot's asset rules, or nil.
func ValidateAsset(asset Asset) error {
	if !assetKeyPattern.MatchString(asset.Key) {
		return fmt.Errorf("asset key %q is not an identifier of at most 64 characters", asset.Key)
	}
	rule, ok := assetRules[asset.Type]
	if !ok {
		return fmt.Errorf("asset %s has unknown type %q", asset.Key, asset.Type)
	}
	if !rule.mimeTypes[asset.MIMEType] {
		return fmt.Errorf("asset %s has MIME type %q, not accepted for %s", asset.Key, asset.MIMEType, asset.Type)
	}
	if asset.Size <= 0 || asset.Size > rule.maxBytes {
		return fmt.Errorf("asset %s weighs %d bytes, outside 1..%d for %s", asset.Key, asset.Size, rule.maxBytes, asset.Type)
	}
	if !assetSHA256Pattern.MatchString(asset.SHA256) {
		return fmt.Errorf("asset %s has SHA-256 %q, not 64 lowercase hex characters", asset.Key, asset.SHA256)
	}
	return nil
}

// ValidateAssets reports why assets, a version's set, break Playhoot's asset
// rules: each asset's, unique keys and the total weight; nil when none.
func ValidateAssets(assets []Asset) error {
	keys := make(map[string]bool, len(assets))
	var total int64
	for _, asset := range assets {
		if err := ValidateAsset(asset); err != nil {
			return err
		}
		if keys[asset.Key] {
			return fmt.Errorf("asset key %s is repeated", asset.Key)
		}
		keys[asset.Key] = true
		total += asset.Size
	}
	if total > MaxAssetsBytes {
		return fmt.Errorf("assets weigh %d bytes together, more than %d", total, MaxAssetsBytes)
	}
	return nil
}
