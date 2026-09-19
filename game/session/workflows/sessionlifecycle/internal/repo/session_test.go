package repo

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRepoCreateSessionWithHost(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)

	now := time.Now().UTC()
	lobbyExpiresAt := now.Add(10 * time.Minute)
	gameDefinitionUUID := uuid.NewString()
	hostUserUUID := uuid.NewString()

	created, err := r.CreateSessionWithHost(context.Background(), db, gameDefinitionUUID, hostUserUUID, lobbyExpiresAt)
	require.NoError(t, err)

	require.NotEmpty(t, created.SessionUUID)
	require.NotZero(t, created.HostActorID)
	require.Equal(t, lobbyExpiresAt, created.LobbyExpiresAt)

	var row struct {
		Phase              string
		GameDefinitionUUID string
		HostActorID        *uint
	}
	require.NoError(t, db.Raw(`
		SELECT phase, game_definition_uuid, host_actor_id
		FROM sessions WHERE id = ?
	`, created.SessionID).Scan(&row).Error)
	require.Equal(t, session.PhaseLobby, row.Phase)
	require.Equal(t, gameDefinitionUUID, row.GameDefinitionUUID)
	require.NotNil(t, row.HostActorID)
	require.Equal(t, created.HostActorID, *row.HostActorID)

	var actorCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_actors WHERE session_id = ? AND user_uuid = ?`, created.SessionID, hostUserUUID).Scan(&actorCount).Error)
	require.Equal(t, int64(1), actorCount)

	var participantCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM session_participants p
		INNER JOIN session_actors a ON a.id = p.session_actor_id
		WHERE a.session_id = ?
	`, created.SessionID).Scan(&participantCount).Error)
	require.Equal(t, int64(0), participantCount, "CreateSessionWithHost must never create a Participant for the host")
}

func TestRepoSetSessionTerminal(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)

	now := time.Now().UTC()
	created, err := r.CreateSessionWithHost(context.Background(), db, uuid.NewString(), uuid.NewString(), now.Add(10*time.Minute))
	require.NoError(t, err)

	terminalAt := now.Add(-1 * time.Minute)
	require.NoError(t, r.SetSessionTerminal(context.Background(), db, created.SessionID, terminalAt, session.TerminalReasonLobbyExpired))

	var row struct {
		Phase          string
		TerminalReason *string
	}
	require.NoError(t, db.Raw(`SELECT phase, terminal_reason FROM sessions WHERE id = ?`, created.SessionID).Scan(&row).Error)
	require.Equal(t, session.PhaseTerminal, row.Phase)
	require.NotNil(t, row.TerminalReason)
	require.Equal(t, session.TerminalReasonLobbyExpired, *row.TerminalReason)
}
