package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

type seededSession struct {
	id            uint
	uuid          string
	gameVersionID uint
}

func seedSession(t *testing.T, db *gorm.DB) seededSession {
	t.Helper()
	session := seededSession{uuid: uuid.NewString(), gameVersionID: uint(time.Now().UnixNano() % 1_000_000)}
	require.NoError(t, db.Raw(`
		INSERT INTO play_sessions (uuid, game_version_id, created_by_user_uuid, status, lobby_expires_at, expires_at)
		VALUES (?, ?, ?, 'RUNNING', NOW(), NOW() + INTERVAL '1 hour') RETURNING id
	`, session.uuid, session.gameVersionID, uuid.NewString()).Scan(&session.id).Error)
	return session
}

// manifestRow and chunkRow are what the tables hold for a Session.
type manifestRow struct {
	GameVersionID      uint
	FinalSessionTimeMs *int64
	FinalizedAt        *time.Time
}

type chunkRow struct {
	RecordingUUID        string
	RecordingStartedAt   time.Time
	ChunkIndex           int
	FormatVersion        int
	FromSessionTimeMs    int64
	ThroughSessionTimeMs int64
	Final                bool
	ObjectKey            string
	Sha256               string
	Size                 int64
}

func TestRegisterChunk(t *testing.T) {
	type test struct {
		sessionUUID string
		chunk       Chunk

		expectedFound    bool
		expectedManifest []manifestRow
		expectedChunks   []chunkRow
	}

	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC)
	newChunk := func(recordingUUID string, index int, through int64, final bool) Chunk {
		return Chunk{
			RecordingUUID: recordingUUID, RecordingStartedAt: startedAt, Index: index, FormatVersion: 1,
			FromSessionTimeMs: int64(index) * 60_000, ThroughSessionTimeMs: through, Final: final,
			ObjectKey: "replays/key", SHA256: "abc", Size: 10,
		}
	}
	rowOf := func(chunk Chunk) chunkRow {
		return chunkRow{
			RecordingUUID: chunk.RecordingUUID, RecordingStartedAt: chunk.RecordingStartedAt, ChunkIndex: chunk.Index, FormatVersion: chunk.FormatVersion,
			FromSessionTimeMs: chunk.FromSessionTimeMs, ThroughSessionTimeMs: chunk.ThroughSessionTimeMs, Final: chunk.Final,
			ObjectKey: chunk.ObjectKey, Sha256: chunk.SHA256, Size: chunk.Size,
		}
	}
	register := func(t *testing.T, db *gorm.DB, sessionUUID string, chunk Chunk) {
		found, err := New(db).RegisterChunk(context.Background(), sessionUUID, chunk, at)
		require.NoError(t, err)
		require.True(t, found)
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"first_chunk_creates_the_manifest_with_the_pinned_version": func(t *testing.T, db *gorm.DB) test {
			session, recording := seedSession(t, db), uuid.NewString()
			chunk := newChunk(recording, 0, 60_000, false)
			return test{
				sessionUUID:      session.uuid,
				chunk:            chunk,
				expectedFound:    true,
				expectedManifest: []manifestRow{{GameVersionID: session.gameVersionID}},
				expectedChunks:   []chunkRow{rowOf(chunk)},
			}
		},
		"chunk_registered_again_changes_nothing": func(t *testing.T, db *gorm.DB) test {
			session, recording := seedSession(t, db), uuid.NewString()
			chunk := newChunk(recording, 0, 60_000, false)
			register(t, db, session.uuid, chunk)
			return test{
				sessionUUID:      session.uuid,
				chunk:            chunk,
				expectedFound:    true,
				expectedManifest: []manifestRow{{GameVersionID: session.gameVersionID}},
				expectedChunks:   []chunkRow{rowOf(chunk)},
			}
		},
		"final_chunk_finalizes_the_manifest_once": func(t *testing.T, db *gorm.DB) test {
			session, recording := seedSession(t, db), uuid.NewString()
			first := newChunk(recording, 0, 60_000, false)
			final := newChunk(recording, 1, 77_400, true)
			register(t, db, session.uuid, first)
			register(t, db, session.uuid, final)
			again := final
			again.ThroughSessionTimeMs = 99_000
			finalTime := int64(77_400)
			return test{
				sessionUUID:      session.uuid,
				chunk:            again,
				expectedFound:    true,
				expectedManifest: []manifestRow{{GameVersionID: session.gameVersionID, FinalSessionTimeMs: &finalTime, FinalizedAt: &at}},
				expectedChunks:   []chunkRow{rowOf(first), rowOf(final)},
			}
		},
		"unknown_session_stores_nothing": func(t *testing.T, db *gorm.DB) test {
			return test{sessionUUID: uuid.NewString(), chunk: newChunk(uuid.NewString(), 0, 60_000, false)}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			found, err := repo.RegisterChunk(context.Background(), tc.sessionUUID, tc.chunk, at)

			require.NoError(t, err)
			require.Equal(t, tc.expectedFound, found)
			var manifest []manifestRow
			require.NoError(t, db.Raw(`
				SELECT r.game_version_id, r.final_session_time_ms, r.finalized_at
				FROM play_replays r JOIN play_sessions s ON s.id = r.session_id WHERE s.uuid = ?
			`, tc.sessionUUID).Scan(&manifest).Error)
			for i := range manifest {
				if manifest[i].FinalizedAt != nil {
					utc := manifest[i].FinalizedAt.UTC()
					manifest[i].FinalizedAt = &utc
				}
			}
			require.Equal(t, tc.expectedManifest, manifest)
			var chunks []chunkRow
			require.NoError(t, db.Raw(`
				SELECT c.recording_uuid, c.recording_started_at, c.chunk_index, c.format_version, c.from_session_time_ms,
					c.through_session_time_ms, c.final, c.object_key, c.sha256, c.size
				FROM play_replay_chunks c JOIN play_sessions s ON s.id = c.session_id WHERE s.uuid = ? ORDER BY c.chunk_index
			`, tc.sessionUUID).Scan(&chunks).Error)
			for i := range chunks {
				chunks[i].RecordingStartedAt = chunks[i].RecordingStartedAt.UTC()
			}
			require.Equal(t, tc.expectedChunks, chunks)
		})
	}
}
