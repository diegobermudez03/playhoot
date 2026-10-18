package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// startableSession is a Session seeded for the Start integration tests, in a
// LOBBY of its own Game, with an active join code.
type startableSession struct {
	ID          uint
	UUID        string
	CreatorUUID string
	VersionUUID string
	JoinCode    string
}

const startableScript = `function start(participants) { return { state: {} }; }`

// seedStartableSession creates a Session in status whose lobby expires at
// lobbyExpiresAt, pinned to an EXECUTABLE version whose script is in store.
func seedStartableSession(t *testing.T, db *gorm.DB, store objectstore.Store, status string, lobbyExpiresAt time.Time) startableSession {
	t.Helper()
	locator, err := objectstore.PutContent(context.Background(), store, "game-versions/backend-scripts", []byte(startableScript), "text/javascript")
	require.NoError(t, err)
	game := integrationGame{GameUUID: uuid.NewString()}
	require.NoError(t, db.Create(&game).Error)
	versionUUID := uuid.NewString()
	require.NoError(t, db.Exec(`
		INSERT INTO play_game_versions (uuid, game_id, status, manifest, script_object_key, script_sha256, script_size)
		VALUES (?, ?, 'EXECUTABLE', '{"schema_version":1,"players":{"min":1,"max":4}}', ?, ?, ?)
	`, versionUUID, game.ID, locator.ObjectKey, locator.SHA256, locator.Size).Error)
	var versionID uint
	require.NoError(t, db.Raw(`SELECT id FROM play_game_versions WHERE uuid = ?`, versionUUID).Scan(&versionID).Error)
	session := integrationSession{
		UUID:              uuid.NewString(),
		GameVersionID:     versionID,
		CreatedByUserUUID: uuid.NewString(),
		Status:            status,
		LobbyExpiresAt:    lobbyExpiresAt,
	}
	require.NoError(t, db.Create(&session).Error)
	code := integrationCode()
	require.NoError(t, db.Create(&integrationJoinCode{SessionID: session.ID, Code: code, ExpiresAt: lobbyExpiresAt}).Error)
	return startableSession{ID: session.ID, UUID: session.UUID, CreatorUUID: session.CreatedByUserUUID, VersionUUID: versionUUID, JoinCode: code}
}

type integrationParticipant struct {
	UserUUID      string
	ParticipantID string
}

type integrationCheckpoint struct {
	ContextRevision int64
	Content         []byte
}

type integrationStatus struct {
	Status   string
	EndCause *string
}

// startEffects reads what Start wrote for sessionID.
func startEffects(t *testing.T, db *gorm.DB, sessionID uint) (integrationStatus, []integrationParticipant, []integrationCheckpoint, []integrationJoinCode) {
	t.Helper()
	var status integrationStatus
	require.NoError(t, db.Raw(`SELECT status, end_cause FROM play_sessions WHERE id = ?`, sessionID).Scan(&status).Error)
	var participants []integrationParticipant
	require.NoError(t, db.Raw(`SELECT user_uuid, participant_id FROM play_session_participants WHERE session_id = ? ORDER BY id`, sessionID).Scan(&participants).Error)
	var checkpoints []integrationCheckpoint
	require.NoError(t, db.Raw(`SELECT context_revision, content FROM play_session_checkpoints WHERE session_id = ?`, sessionID).Scan(&checkpoints).Error)
	var active []integrationJoinCode
	require.NoError(t, db.Raw(`SELECT session_id, code FROM play_session_join_codes WHERE session_id = ? AND released_at IS NULL`, sessionID).Scan(&active).Error)
	return status, participants, checkpoints, active
}

func TestCheckStartEligibilityIntegration(t *testing.T) {
	type test struct {
		sessionUUID string
		userUUID    string
		playerCount int

		expected sessionpkg.StartEligibility
	}

	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	now := time.Now().UTC()

	tests := map[string]func(t *testing.T) test{
		"lobby_is_eligible_with_its_script_from_storage": func(t *testing.T) test {
			session := seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour))
			return test{
				sessionUUID: session.UUID, userUUID: session.CreatorUUID, playerCount: 2,
				expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartEligible, GameVersionUUID: session.VersionUUID, Script: []byte(startableScript)},
			}
		},
		"expired_lobby": func(t *testing.T) test {
			session := seedStartableSession(t, db, store, "LOBBY", now.Add(-time.Minute))
			return test{sessionUUID: session.UUID, userUUID: session.CreatorUUID, playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotInLobby}}
		},
		"too_many_players": func(t *testing.T) test {
			session := seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour))
			return test{sessionUUID: session.UUID, userUUID: session.CreatorUUID, playerCount: 5, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartInvalidPlayerCount}}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			manager := New(db, store)

			eligibility, err := manager.CheckStartEligibility(context.Background(), tc.sessionUUID, tc.userUUID, tc.playerCount)

			require.NoError(t, err)
			require.Equal(t, tc.expected, eligibility)
		})
	}
}

func TestCommitStartIntegration(t *testing.T) {
	type test struct {
		session startableSession
		commit  sessionpkg.StartCommit

		expected             sessionpkg.CommitStartOutcome
		expectErr            bool
		expectedStatus       integrationStatus
		expectedParticipants []integrationParticipant
		expectedCheckpoints  []integrationCheckpoint
		expectedActiveCodes  int
	}

	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	now := time.Now().UTC()
	checkpoint := sessionpkg.Checkpoint{ContextRevision: 1, Content: []byte(`{"schema_version":1}`)}

	tests := map[string]func(t *testing.T) test{
		"lobby_becomes_running_with_roster_and_checkpoint": func(t *testing.T) test {
			session := seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour))
			userA, userB := uuid.NewString(), uuid.NewString()
			return test{
				session: session,
				commit: sessionpkg.StartCommit{
					Participants: []sessionpkg.StartParticipant{{UserUUID: userA, ParticipantID: "P1"}, {UserUUID: userB, ParticipantID: "P2"}},
					Checkpoint:   checkpoint,
				},
				expected:             sessionpkg.CommitStartCommitted,
				expectedStatus:       integrationStatus{Status: "RUNNING"},
				expectedParticipants: []integrationParticipant{{UserUUID: userA, ParticipantID: "P1"}, {UserUUID: userB, ParticipantID: "P2"}},
				expectedCheckpoints:  []integrationCheckpoint{{ContextRevision: 1, Content: []byte(`{"schema_version":1}`)}},
			}
		},
		"expired_lobby_records_nothing": func(t *testing.T) test {
			session := seedStartableSession(t, db, store, "LOBBY", now.Add(-time.Minute))
			return test{
				session: session,
				commit: sessionpkg.StartCommit{
					Participants: []sessionpkg.StartParticipant{{UserUUID: uuid.NewString(), ParticipantID: "P1"}},
					Checkpoint:   checkpoint,
				},
				expected:            sessionpkg.CommitStartNotInLobby,
				expectedStatus:      integrationStatus{Status: "LOBBY"},
				expectedActiveCodes: 1,
			}
		},
		"failure_rolls_back_everything": func(t *testing.T) test {
			session := seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour))
			taken := uuid.NewString()
			// Broken data: P1 is already taken, so recording the roster fails.
			require.NoError(t, db.Exec(`INSERT INTO play_session_participants (session_id, user_uuid, participant_id) VALUES (?, ?, 'P1')`, session.ID, taken).Error)
			return test{
				session: session,
				commit: sessionpkg.StartCommit{
					Participants: []sessionpkg.StartParticipant{{UserUUID: uuid.NewString(), ParticipantID: "P1"}},
					Checkpoint:   checkpoint,
				},
				expectErr:            true,
				expectedStatus:       integrationStatus{Status: "LOBBY"},
				expectedParticipants: []integrationParticipant{{UserUUID: taken, ParticipantID: "P1"}},
				expectedActiveCodes:  1,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			manager := New(db, store)

			outcome, err := manager.CommitStart(context.Background(), tc.session.UUID, tc.session.CreatorUUID, tc.commit)

			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, outcome)
			}
			status, participants, checkpoints, active := startEffects(t, db, tc.session.ID)
			require.Equal(t, tc.expectedStatus, status)
			require.Equal(t, tc.expectedParticipants, participants)
			require.Equal(t, tc.expectedCheckpoints, checkpoints)
			require.Len(t, active, tc.expectedActiveCodes)
		})
	}
}

func TestAbortStartIntegration(t *testing.T) {
	type test struct {
		session startableSession

		expectedStatus      integrationStatus
		expectedActiveCodes int
	}

	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	now := time.Now().UTC()
	cause := "GAME_START_FAILURE"

	tests := map[string]func(t *testing.T) test{
		"lobby_is_aborted_and_its_code_released": func(t *testing.T) test {
			return test{
				session:        seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour)),
				expectedStatus: integrationStatus{Status: "ABORTED", EndCause: &cause},
			}
		},
		"running_session_is_left_as_it_is": func(t *testing.T) test {
			return test{
				session:             seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				expectedStatus:      integrationStatus{Status: "RUNNING"},
				expectedActiveCodes: 1,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			manager := New(db, store)

			err := manager.AbortStart(context.Background(), tc.session.UUID)

			require.NoError(t, err)
			status, _, _, active := startEffects(t, db, tc.session.ID)
			require.Equal(t, tc.expectedStatus, status)
			require.Len(t, active, tc.expectedActiveCodes)
		})
	}
}
