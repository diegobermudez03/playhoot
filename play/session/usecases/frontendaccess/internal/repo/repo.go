// Package repo is frontendaccess's persistence: it reads where a Session's
// game frontend and its assets are stored, and keeps each version's current
// signed URLs; it decides nothing itself.
package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is frontendaccess's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// Frontend is a Session's lifetime and its pinned game version: the
// version's status, the locator of its frontend package and the aspect ratio
// it is shown in. The version's fields are nil when it has none, or when the
// pinned version does not exist (GameVersionID nil).
type Frontend struct {
	SessionExpiresAt time.Time
	GameVersionID    *int64
	Status           *string
	ObjectKey        *string
	SHA256           *string
	AspectRatio      *string
}

// FindFrontend returns sessionUUID's frontend, or nil when there is no such
// Session.
func (r *Repo) FindFrontend(ctx context.Context, sessionUUID string) (*Frontend, error) {
	var frontend Frontend
	result := r.db.WithContext(ctx).Raw(`
		SELECT s.expires_at AS session_expires_at, v.id AS game_version_id, v.status,
			v.frontend_object_key AS object_key, v.frontend_sha256 AS sha256, v.aspect_ratio
		FROM play_sessions s
		LEFT JOIN play_game_versions v ON v.id = s.game_version_id
		WHERE s.uuid = ?
	`, sessionUUID).Scan(&frontend)
	if result.Error != nil {
		return nil, fmt.Errorf("finding frontend package: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &frontend, nil
}

// Asset is one asset of a game version. ObjectKey is nil when its object is
// no longer stored.
type Asset struct {
	Key       string
	Type      string
	MIMEType  string
	ObjectKey *string
	SHA256    string
	Size      int64
}

// FindAssets returns every asset of gameVersionID, by key.
func (r *Repo) FindAssets(ctx context.Context, gameVersionID int64) ([]Asset, error) {
	var assets []Asset
	if err := r.db.WithContext(ctx).Raw(`
		SELECT key, type, mime_type, object_key, sha256, size
		FROM play_game_version_assets
		WHERE game_version_id = ?
		ORDER BY key
	`, gameVersionID).Scan(&assets).Error; err != nil {
		return nil, fmt.Errorf("finding game version assets: %w", err)
	}
	return assets, nil
}

// SignedURLs is a batch of signed URLs to a game version's frontend package
// and to its assets by key, all expiring at ExpiresAt.
type SignedURLs struct {
	FrontendURL string
	AssetURLs   map[string]string
	ExpiresAt   time.Time
}

// signedURLsRow is how SignedURLs is stored.
type signedURLsRow struct {
	FrontendURL string
	AssetURLs   []byte
	ExpiresAt   time.Time
}

// FindSignedURLs returns gameVersionID's current batch, or nil when it has
// none.
func (r *Repo) FindSignedURLs(ctx context.Context, gameVersionID int64) (*SignedURLs, error) {
	var row signedURLsRow
	result := r.db.WithContext(ctx).Raw(`
		SELECT frontend_url, asset_urls, expires_at
		FROM play_game_version_signed_urls
		WHERE game_version_id = ?
	`, gameVersionID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("finding signed urls: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	urls := SignedURLs{FrontendURL: row.FrontendURL, ExpiresAt: row.ExpiresAt}
	if err := json.Unmarshal(row.AssetURLs, &urls.AssetURLs); err != nil {
		return nil, fmt.Errorf("decoding signed asset urls: %w", err)
	}
	return &urls, nil
}

// SaveSignedURLs makes urls gameVersionID's current batch, unless the current
// one expires later: concurrent signers each keep a valid batch, and the
// longest-lived one stays.
func (r *Repo) SaveSignedURLs(ctx context.Context, gameVersionID int64, urls SignedURLs, signedAt time.Time) error {
	assetURLs := urls.AssetURLs
	if assetURLs == nil {
		assetURLs = map[string]string{}
	}
	encoded, err := json.Marshal(assetURLs)
	if err != nil {
		return fmt.Errorf("encoding signed asset urls: %w", err)
	}
	if err := r.db.WithContext(ctx).Exec(`
		INSERT INTO play_game_version_signed_urls (game_version_id, frontend_url, asset_urls, expires_at, signed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (game_version_id) DO UPDATE SET
			frontend_url = EXCLUDED.frontend_url,
			asset_urls = EXCLUDED.asset_urls,
			expires_at = EXCLUDED.expires_at,
			signed_at = EXCLUDED.signed_at
		WHERE play_game_version_signed_urls.expires_at < EXCLUDED.expires_at
	`, gameVersionID, urls.FrontendURL, string(encoded), urls.ExpiresAt, signedAt).Error; err != nil {
		return fmt.Errorf("saving signed urls: %w", err)
	}
	return nil
}
