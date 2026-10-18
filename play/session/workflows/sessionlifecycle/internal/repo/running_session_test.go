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

func TestFindRunningSession(t *testing.T) {
	type test struct {
		sessionUUID string

		expected *RunningSessionRecord
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"session_with_its_checkpoint": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			require.NoError(t, New(db).SaveCheckpoint(context.Background(), db, created.ID, 3, []byte(`{"round":2}`), time.Now()))
			var versionUUID string
			require.NoError(t, db.Raw(`SELECT uuid FROM play_game_versions WHERE id = ?`, created.GameVersionID).Scan(&versionUUID).Error)
			revision := int64(3)
			return test{
				sessionUUID: created.UUID,
				expected:    &RunningSessionRecord{ID: created.ID, Status: "LOBBY", GameVersionUUID: versionUUID, CheckpointRevision: &revision, CheckpointContent: []byte(`{"round":2}`)},
			}
		},
		"session_without_checkpoint": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			var versionUUID string
			require.NoError(t, db.Raw(`SELECT uuid FROM play_game_versions WHERE id = ?`, created.GameVersionID).Scan(&versionUUID).Error)
			return test{sessionUUID: created.UUID, expected: &RunningSessionRecord{ID: created.ID, Status: "LOBBY", GameVersionUUID: versionUUID}}
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

			record, err := repo.FindRunningSession(context.Background(), tc.sessionUUID)

			require.NoError(t, err)
			require.Equal(t, tc.expected, record)
		})
	}
}

func TestFindParticipants(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	created := seedSession(t, db, time.Now().Add(time.Hour))
	roster := []Participant{{UserUUID: uuid.NewString(), ParticipantID: "P1"}, {UserUUID: uuid.NewString(), ParticipantID: "P2"}}
	require.NoError(t, repo.CreateParticipants(context.Background(), db, created.ID, roster))

	participants, err := repo.FindParticipants(context.Background(), created.ID)

	require.NoError(t, err)
	require.Equal(t, roster, participants, "in the order they were recorded")
}
