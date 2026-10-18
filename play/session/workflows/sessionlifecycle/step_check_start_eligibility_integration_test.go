package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

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
