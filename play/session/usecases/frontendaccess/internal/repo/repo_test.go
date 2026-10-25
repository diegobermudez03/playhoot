package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// seedVersion creates a Game with one version in status, whose frontend
// package is at key with hash sha when key is not nil, shown in aspectRatio
// when it is not nil, and returns the version's id.
func seedVersion(t *testing.T, db *gorm.DB, status string, key *string, sha *string, aspectRatio *string) int64 {
	t.Helper()
	var gameID uint
	require.NoError(t, db.Raw(`INSERT INTO play_games (game_uuid) VALUES (?) RETURNING id`, uuid.NewString()).Scan(&gameID).Error)
	var versionID int64
	require.NoError(t, db.Raw(`
		INSERT INTO play_game_versions (uuid, game_id, status, manifest, frontend_object_key, frontend_sha256, frontend_size, aspect_ratio)
		VALUES (?, ?, ?, '{"schema_version":1,"players":{"min":1,"max":8}}', ?, ?, 10, ?)
		RETURNING id
	`, uuid.NewString(), gameID, status, key, sha, aspectRatio).Scan(&versionID).Error)
	return versionID
}

// seedSession creates a LOBBY Session pinned to versionID expiring at
// expiresAt, and returns its UUID.
func seedSession(t *testing.T, db *gorm.DB, versionID int64, expiresAt time.Time) string {
	t.Helper()
	sessionUUID := uuid.NewString()
	require.NoError(t, db.Exec(`
		INSERT INTO play_sessions (uuid, game_version_id, created_by_user_uuid, status, lobby_expires_at, expires_at)
		VALUES (?, ?, ?, 'LOBBY', ?, ?)
	`, sessionUUID, versionID, uuid.NewString(), expiresAt, expiresAt).Error)
	return sessionUUID
}

// sha is a content hash made of one repeated hex digit.
func sha(digit string) string { return strings.Repeat(digit, 64) }

func TestFindFrontend(t *testing.T) {
	type test struct {
		sessionUUID string

		expected *Frontend
	}

	expiresAt := time.Date(2026, 10, 25, 16, 0, 0, 0, time.UTC)
	executable, draft := "EXECUTABLE", "DRAFT"

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"session_of_a_version_with_its_frontend_package": func(t *testing.T, db *gorm.DB) test {
			key, hash, square := "game-versions/frontend-packages/"+uuid.NewString(), "9f86d081", "1:1"
			versionID := seedVersion(t, db, executable, &key, &hash, &square)
			return test{
				sessionUUID: seedSession(t, db, versionID, expiresAt),
				expected:    &Frontend{SessionExpiresAt: expiresAt, GameVersionID: &versionID, Status: &executable, ObjectKey: &key, SHA256: &hash, AspectRatio: &square},
			}
		},
		"session_of_a_version_without_frontend_package": func(t *testing.T, db *gorm.DB) test {
			versionID := seedVersion(t, db, draft, nil, nil, nil)
			return test{
				sessionUUID: seedSession(t, db, versionID, expiresAt),
				expected:    &Frontend{SessionExpiresAt: expiresAt, GameVersionID: &versionID, Status: &draft},
			}
		},
		"session_pinned_to_a_missing_version": func(t *testing.T, db *gorm.DB) test {
			return test{sessionUUID: seedSession(t, db, -1, expiresAt), expected: &Frontend{SessionExpiresAt: expiresAt}}
		},
		"unknown_session": func(t *testing.T, db *gorm.DB) test {
			return test{sessionUUID: uuid.NewString()}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			frontend, err := repo.FindFrontend(context.Background(), tc.sessionUUID)

			require.NoError(t, err)
			if tc.expected != nil {
				require.NotNil(t, frontend)
				frontend.SessionExpiresAt = frontend.SessionExpiresAt.UTC()
			}
			require.Equal(t, tc.expected, frontend)
		})
	}
}

func TestAspectRatioIsOneOfTheOffered(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	var gameID uint
	require.NoError(t, db.Raw(`INSERT INTO play_games (game_uuid) VALUES (?) RETURNING id`, uuid.NewString()).Scan(&gameID).Error)

	err := db.Exec(`
		INSERT INTO play_game_versions (uuid, game_id, status, manifest, aspect_ratio)
		VALUES (?, ?, 'DRAFT', '{"schema_version":1,"players":{"min":1,"max":8}}', '21:9')
	`, uuid.NewString(), gameID).Error

	require.ErrorContains(t, err, "play_game_versions_aspect_ratio_check")
}

// insertAsset stores an asset of versionID.
func insertAsset(db *gorm.DB, versionID int64, key, assetType, mimeType string, objectKey *string, hash string, size int64) error {
	return db.Exec(`
		INSERT INTO play_game_version_assets (game_version_id, key, type, mime_type, object_key, sha256, size)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, versionID, key, assetType, mimeType, objectKey, hash, size).Error
}

func TestFindAssets(t *testing.T) {
	type test struct {
		gameVersionID int64

		expected []Asset
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"assets_by_key_with_and_without_object": func(t *testing.T, db *gorm.DB) test {
			versionID := seedVersion(t, db, "EXECUTABLE", nil, nil, nil)
			shipKey := "game-versions/assets/" + sha("a")
			require.NoError(t, insertAsset(db, versionID, "thrust", "audio", "audio/wav", nil, sha("b"), 4844))
			require.NoError(t, insertAsset(db, versionID, "ship", "image", "image/png", &shipKey, sha("a"), 175))
			other := seedVersion(t, db, "EXECUTABLE", nil, nil, nil)
			require.NoError(t, insertAsset(db, other, "ship", "image", "image/webp", &shipKey, sha("a"), 175))
			return test{
				gameVersionID: versionID,
				expected: []Asset{
					{Key: "ship", Type: "image", MIMEType: "image/png", ObjectKey: &shipKey, SHA256: sha("a"), Size: 175},
					{Key: "thrust", Type: "audio", MIMEType: "audio/wav", SHA256: sha("b"), Size: 4844},
				},
			}
		},
		"version_without_assets": func(t *testing.T, db *gorm.DB) test {
			return test{gameVersionID: seedVersion(t, db, "EXECUTABLE", nil, nil, nil)}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			assets, err := repo.FindAssets(context.Background(), tc.gameVersionID)

			require.NoError(t, err)
			require.Equal(t, tc.expected, assets)
		})
	}
}

func TestAssetKeysAreUniquePerVersion(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	versionID := seedVersion(t, db, "DRAFT", nil, nil, nil)
	require.NoError(t, insertAsset(db, versionID, "ship", "image", "image/png", nil, sha("a"), 1))

	err := insertAsset(db, versionID, "ship", "audio", "audio/wav", nil, sha("b"), 1)

	require.ErrorContains(t, err, "play_game_version_assets_game_version_id_key_key")
}

func TestFindSignedURLs(t *testing.T) {
	type test struct {
		gameVersionID int64

		expected *SignedURLs
	}

	expiresAt := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"version_with_a_batch": func(t *testing.T, db *gorm.DB) test {
			versionID := seedVersion(t, db, "EXECUTABLE", nil, nil, nil)
			require.NoError(t, db.Exec(`
				INSERT INTO play_game_version_signed_urls (game_version_id, frontend_url, asset_urls, expires_at, signed_at)
				VALUES (?, 'https://storage.example/f', '{"ship":"https://storage.example/ship"}', ?, ?)
			`, versionID, expiresAt, expiresAt.Add(-24*time.Hour)).Error)
			return test{
				gameVersionID: versionID,
				expected:      &SignedURLs{FrontendURL: "https://storage.example/f", AssetURLs: map[string]string{"ship": "https://storage.example/ship"}, ExpiresAt: expiresAt},
			}
		},
		"version_without_a_batch": func(t *testing.T, db *gorm.DB) test {
			return test{gameVersionID: seedVersion(t, db, "EXECUTABLE", nil, nil, nil)}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			urls, err := repo.FindSignedURLs(context.Background(), tc.gameVersionID)

			require.NoError(t, err)
			if tc.expected != nil {
				require.NotNil(t, urls)
				urls.ExpiresAt = urls.ExpiresAt.UTC()
			}
			require.Equal(t, tc.expected, urls)
		})
	}
}

// storedBatch is a version's stored batch, read as stored.
type storedBatch struct {
	FrontendURL string
	AssetURLs   map[string]string
	ExpiresAt   time.Time
	SignedAt    time.Time
}

func TestSaveSignedURLs(t *testing.T) {
	type test struct {
		gameVersionID int64
		urls          SignedURLs

		expected storedBatch
	}

	signedAt := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	earlierSignedAt := signedAt.Add(-time.Hour)
	batch := func(name string, expiresAt time.Time) SignedURLs {
		return SignedURLs{
			FrontendURL: fmt.Sprintf("https://storage.example/f?sig=%s", name),
			AssetURLs:   map[string]string{"ship": fmt.Sprintf("https://storage.example/ship?sig=%s", name)},
			ExpiresAt:   expiresAt,
		}
	}
	older, newer := batch("older", signedAt.Add(23*time.Hour)), batch("newer", signedAt.Add(24*time.Hour))
	stored := func(urls SignedURLs, at time.Time) storedBatch {
		return storedBatch{FrontendURL: urls.FrontendURL, AssetURLs: urls.AssetURLs, ExpiresAt: urls.ExpiresAt, SignedAt: at}
	}

	tests := map[string]func(t *testing.T, db *gorm.DB, repo *Repo) test{
		"first_batch_of_a_version": func(t *testing.T, db *gorm.DB, repo *Repo) test {
			return test{gameVersionID: seedVersion(t, db, "EXECUTABLE", nil, nil, nil), urls: newer, expected: stored(newer, signedAt)}
		},
		"a_later_expiring_batch_replaces_the_current_one": func(t *testing.T, db *gorm.DB, repo *Repo) test {
			versionID := seedVersion(t, db, "EXECUTABLE", nil, nil, nil)
			require.NoError(t, repo.SaveSignedURLs(context.Background(), versionID, older, earlierSignedAt))
			return test{gameVersionID: versionID, urls: newer, expected: stored(newer, signedAt)}
		},
		"an_earlier_expiring_batch_keeps_the_current_one": func(t *testing.T, db *gorm.DB, repo *Repo) test {
			versionID := seedVersion(t, db, "EXECUTABLE", nil, nil, nil)
			require.NoError(t, repo.SaveSignedURLs(context.Background(), versionID, newer, earlierSignedAt))
			return test{gameVersionID: versionID, urls: older, expected: stored(newer, earlierSignedAt)}
		},
		"a_batch_without_assets": func(t *testing.T, db *gorm.DB, repo *Repo) test {
			urls := SignedURLs{FrontendURL: "https://storage.example/f", ExpiresAt: signedAt.Add(time.Hour)}
			return test{
				gameVersionID: seedVersion(t, db, "EXECUTABLE", nil, nil, nil),
				urls:          urls,
				expected:      storedBatch{FrontendURL: urls.FrontendURL, AssetURLs: map[string]string{}, ExpiresAt: urls.ExpiresAt, SignedAt: signedAt},
			}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db, repo)

			err := repo.SaveSignedURLs(context.Background(), tc.gameVersionID, tc.urls, signedAt)

			require.NoError(t, err)
			var rows []struct {
				FrontendURL string
				AssetURLs   string
				ExpiresAt   time.Time
				SignedAt    time.Time
			}
			require.NoError(t, db.Raw(`
				SELECT frontend_url, asset_urls::text AS asset_urls, expires_at, signed_at
				FROM play_game_version_signed_urls WHERE game_version_id = ?
			`, tc.gameVersionID).Scan(&rows).Error)
			require.Len(t, rows, 1, "one batch per version")
			got := storedBatch{FrontendURL: rows[0].FrontendURL, ExpiresAt: rows[0].ExpiresAt.UTC(), SignedAt: rows[0].SignedAt.UTC()}
			require.NoError(t, json.Unmarshal([]byte(rows[0].AssetURLs), &got.AssetURLs))
			require.Equal(t, tc.expected, got)
		})
	}
}
