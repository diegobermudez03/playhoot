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

type testParticipant struct {
	SessionID     uint
	UserUUID      string
	ParticipantID string
}

type testCheckpoint struct {
	SessionID       uint
	ContextRevision int64
	Content         []byte
}

type testSessionStatus struct {
	Status   string
	EndCause *string
}

func participantsOf(t *testing.T, db *gorm.DB, sessionID uint) []testParticipant {
	t.Helper()
	var participants []testParticipant
	require.NoError(t, db.Raw(`SELECT session_id, user_uuid, participant_id FROM play_session_participants WHERE session_id = ? ORDER BY id`, sessionID).Scan(&participants).Error)
	return participants
}

func checkpointsOf(t *testing.T, db *gorm.DB, sessionID uint) []testCheckpoint {
	t.Helper()
	var checkpoints []testCheckpoint
	require.NoError(t, db.Raw(`SELECT session_id, context_revision, content FROM play_session_checkpoints WHERE session_id = ?`, sessionID).Scan(&checkpoints).Error)
	return checkpoints
}

func statusOf(t *testing.T, db *gorm.DB, sessionID uint) testSessionStatus {
	t.Helper()
	var status testSessionStatus
	require.NoError(t, db.Raw(`SELECT status, end_cause FROM play_sessions WHERE id = ?`, sessionID).Scan(&status).Error)
	return status
}

func TestFindStartCandidate(t *testing.T) {
	type test struct {
		sessionUUID string

		expected *StartCandidate
	}

	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"session_with_its_version_and_script": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, expiresAt)
			key, sha, size := "game-versions/backend-scripts/abc", "abc", int64(42)
			require.NoError(t, db.Exec(`UPDATE play_game_versions SET script_object_key = ?, script_sha256 = ?, script_size = ? WHERE id = ?`, key, sha, size, created.GameVersionID).Error)
			var versionUUID string
			require.NoError(t, db.Raw(`SELECT uuid FROM play_game_versions WHERE id = ?`, created.GameVersionID).Scan(&versionUUID).Error)
			return test{
				sessionUUID: created.UUID,
				expected: &StartCandidate{
					Status:            "LOBBY",
					CreatedByUserUUID: created.CreatedByUserUUID,
					LobbyExpiresAt:    expiresAt,
					GameVersionUUID:   versionUUID,
					VersionStatus:     "EXECUTABLE",
					Manifest:          []byte(`{"players": {"max": 8, "min": 1}, "schema_version": 1}`),
					ScriptObjectKey:   &key,
					ScriptSHA256:      &sha,
					ScriptSize:        &size,
				},
			}
		},
		"version_without_script": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, expiresAt)
			var versionUUID string
			require.NoError(t, db.Raw(`SELECT uuid FROM play_game_versions WHERE id = ?`, created.GameVersionID).Scan(&versionUUID).Error)
			return test{
				sessionUUID: created.UUID,
				expected: &StartCandidate{
					Status:            "LOBBY",
					CreatedByUserUUID: created.CreatedByUserUUID,
					LobbyExpiresAt:    expiresAt,
					GameVersionUUID:   versionUUID,
					VersionStatus:     "EXECUTABLE",
					Manifest:          []byte(`{"players": {"max": 8, "min": 1}, "schema_version": 1}`),
				},
			}
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

			candidate, err := repo.FindStartCandidate(context.Background(), tc.sessionUUID)

			require.NoError(t, err)
			if candidate != nil {
				candidate.LobbyExpiresAt = candidate.LobbyExpiresAt.UTC()
			}
			require.Equal(t, tc.expected, candidate)
		})
	}
}

func TestLockSession(t *testing.T) {
	type test struct {
		sessionUUID string

		expected *LockedSession
	}

	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"existing_session": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, expiresAt)
			return test{
				sessionUUID: created.UUID,
				expected:    &LockedSession{ID: created.ID, Status: "LOBBY", CreatedByUserUUID: created.CreatedByUserUUID, LobbyExpiresAt: expiresAt},
			}
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

			var locked *LockedSession
			err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				locked, err = repo.LockSession(context.Background(), tx, tc.sessionUUID)
				return err
			})

			require.NoError(t, err)
			if locked != nil {
				locked.LobbyExpiresAt = locked.LobbyExpiresAt.UTC()
			}
			require.Equal(t, tc.expected, locked)
		})
	}
}

func TestCreateParticipants(t *testing.T) {
	type test struct {
		sessionID    uint
		participants []Participant

		expectErr            bool
		expectedParticipants []testParticipant
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"every_participant_is_recorded": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			userA, userB := uuid.NewString(), uuid.NewString()
			return test{
				sessionID:    created.ID,
				participants: []Participant{{UserUUID: userA, ParticipantID: "P1"}, {UserUUID: userB, ParticipantID: "P2"}},
				expectedParticipants: []testParticipant{
					{SessionID: created.ID, UserUUID: userA, ParticipantID: "P1"},
					{SessionID: created.ID, UserUUID: userB, ParticipantID: "P2"},
				},
			}
		},
		"repeated_participant_id_is_refused": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			return test{
				sessionID:    created.ID,
				participants: []Participant{{UserUUID: uuid.NewString(), ParticipantID: "P1"}, {UserUUID: uuid.NewString(), ParticipantID: "P1"}},
				expectErr:    true,
			}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			err := db.Transaction(func(tx *gorm.DB) error {
				return repo.CreateParticipants(context.Background(), tx, tc.sessionID, tc.participants)
			})

			if tc.expectErr {
				require.Error(t, err)
				require.Empty(t, participantsOf(t, db, tc.sessionID))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedParticipants, participantsOf(t, db, tc.sessionID))
		})
	}
}

func TestSaveCheckpoint(t *testing.T) {
	type test struct {
		sessionID       uint
		contextRevision int64
		content         []byte

		expectedCheckpoints []testCheckpoint
	}

	savedAt := time.Now().UTC()
	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"first_checkpoint_is_stored": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			return test{
				sessionID: created.ID, contextRevision: 1, content: []byte(`{"round":1}`),
				expectedCheckpoints: []testCheckpoint{{SessionID: created.ID, ContextRevision: 1, Content: []byte(`{"round":1}`)}},
			}
		},
		"newer_checkpoint_replaces_the_stored_one": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			require.NoError(t, New(db).SaveCheckpoint(context.Background(), db, created.ID, 1, []byte(`{"round":1}`), savedAt))
			return test{
				sessionID: created.ID, contextRevision: 5, content: []byte(`{"round":2}`),
				expectedCheckpoints: []testCheckpoint{{SessionID: created.ID, ContextRevision: 5, Content: []byte(`{"round":2}`)}},
			}
		},
		"older_checkpoint_never_replaces_a_newer_one": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			require.NoError(t, New(db).SaveCheckpoint(context.Background(), db, created.ID, 5, []byte(`{"round":2}`), savedAt))
			return test{
				sessionID: created.ID, contextRevision: 1, content: []byte(`{"round":1}`),
				expectedCheckpoints: []testCheckpoint{{SessionID: created.ID, ContextRevision: 5, Content: []byte(`{"round":2}`)}},
			}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			err := repo.SaveCheckpoint(context.Background(), db, tc.sessionID, tc.contextRevision, tc.content, savedAt)

			require.NoError(t, err)
			require.Equal(t, tc.expectedCheckpoints, checkpointsOf(t, db, tc.sessionID))
		})
	}
}

func TestUpdateSessionStatus(t *testing.T) {
	type test struct {
		sessionID uint
		status    string
		endCause  *string

		expectErr      bool
		expectedStatus testSessionStatus
	}

	cause := "GAME_START_FAILURE"
	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"status_without_end_cause": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			return test{sessionID: created.ID, status: "RUNNING", expectedStatus: testSessionStatus{Status: "RUNNING"}}
		},
		"terminal_status_with_its_end_cause": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			return test{sessionID: created.ID, status: "ABORTED", endCause: &cause, expectedStatus: testSessionStatus{Status: "ABORTED", EndCause: &cause}}
		},
		"unknown_session": func(t *testing.T, db *gorm.DB) test {
			return test{sessionID: 0, status: "RUNNING", expectErr: true}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			err := repo.UpdateSessionStatus(context.Background(), db, tc.sessionID, tc.status, tc.endCause, time.Now())

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedStatus, statusOf(t, db, tc.sessionID))
		})
	}
}

func TestReleaseJoinCodes(t *testing.T) {
	type test struct {
		sessionID uint
		code      string
		otherCode string
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"only_the_sessions_active_codes_are_released": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, time.Now().Add(time.Hour))
			other := seedSession(t, db, time.Now().Add(time.Hour))
			code, otherCode := randomCode(), randomCode()
			require.NoError(t, db.Create(&testJoinCode{SessionID: created.ID, Code: code, ExpiresAt: time.Now().Add(time.Hour)}).Error)
			require.NoError(t, db.Create(&testJoinCode{SessionID: other.ID, Code: otherCode, ExpiresAt: time.Now().Add(time.Hour)}).Error)
			return test{sessionID: created.ID, code: code, otherCode: otherCode}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			err := repo.ReleaseJoinCodes(context.Background(), db, tc.sessionID, time.Now())

			require.NoError(t, err)
			require.Empty(t, activeJoinCodes(t, db, tc.code))
			require.Len(t, activeJoinCodes(t, db, tc.otherCode), 1, "another Session's code stays active")
		})
	}
}
