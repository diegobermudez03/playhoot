package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// latestLobby drains the lobby updates next has ready and returns the last
// one's members, or nil when none was ready.
func latestLobby(t *testing.T, next func(context.Context) (runtime.LobbyUpdate, bool)) []string {
	t.Helper()
	var members []string
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		update, ok := next(ctx)
		cancel()
		if !ok {
			return members
		}
		members = []string{}
		for _, member := range update.Members {
			members = append(members, member.UserUUID)
		}
	}
}

// lobbySnapshot returns the users in the lobby, in order.
func (c *Controller) lobbySnapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.lobby...)
}

func endedWith(t *testing.T, ended func(context.Context) (runtime.ConnectionEnd, bool)) runtime.EndReason {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	end, ok := ended(ctx)
	if !ok {
		return ""
	}
	return end.Reason
}

func TestAdmitPlayer(t *testing.T) {
	type test struct {
		maxPlayers int
		// before runs against the Controller before the admission under test.
		before func(t *testing.T, controller *Controller)
		// after runs against the admitted connection, when there is one.
		after func(t *testing.T, controller *Controller, player runtime.PlayerConnection)

		expectedOK          bool
		expectedRejection   runtime.RejectReason
		expectedLobby       []string
		expectedPlayerLobby []string
		expectedEnd         runtime.EndReason
	}

	ctx := context.Background()
	admit := func(t *testing.T, controller *Controller, userUUID string) runtime.PlayerConnection {
		t.Helper()
		player, _, ok := controller.AdmitPlayer(ctx, userUUID)
		require.True(t, ok)
		return player
	}
	// startGame moves the Controller to a running game, as Start will.
	startGame := func(controller *Controller) {
		controller.mu.Lock()
		controller.phase = phaseRunning
		controller.mu.Unlock()
	}

	tests := map[string]func(t *testing.T) test{
		"first_player_joins_the_lobby": func(t *testing.T) test {
			return test{
				maxPlayers:          8,
				expectedOK:          true,
				expectedLobby:       []string{"user-a"},
				expectedPlayerLobby: []string{"user-a"},
			}
		},
		"players_are_listed_in_the_order_they_joined": func(t *testing.T) test {
			return test{
				maxPlayers:          8,
				before:              func(t *testing.T, controller *Controller) { admit(t, controller, "user-b") },
				expectedOK:          true,
				expectedLobby:       []string{"user-b", "user-a"},
				expectedPlayerLobby: []string{"user-b", "user-a"},
			}
		},
		"full_lobby_rejects": func(t *testing.T) test {
			return test{
				maxPlayers:        1,
				before:            func(t *testing.T, controller *Controller) { admit(t, controller, "user-b") },
				expectedRejection: runtime.RejectLobbyFull,
				expectedLobby:     []string{"user-b"},
			}
		},
		"running_game_rejects_new_players": func(t *testing.T) test {
			return test{
				maxPlayers:        8,
				before:            func(t *testing.T, controller *Controller) { startGame(controller) },
				expectedRejection: runtime.RejectSessionStarted,
				expectedLobby:     []string{},
			}
		},
		"disconnecting_from_a_running_game_keeps_the_participant_and_sends_no_lobby_update": func(t *testing.T) test {
			return test{
				maxPlayers: 8,
				after: func(t *testing.T, controller *Controller, player runtime.PlayerConnection) {
					startGame(controller)
					player.Disconnect()
				},
				expectedOK:          true,
				expectedLobby:       []string{"user-a"},
				expectedPlayerLobby: []string{"user-a"},
				expectedEnd:         runtime.EndDisconnected,
			}
		},
		"leaving_or_commanding_in_a_running_game_is_not_built_yet": func(t *testing.T) test {
			return test{
				maxPlayers: 8,
				after: func(t *testing.T, controller *Controller, player runtime.PlayerConnection) {
					startGame(controller)
					_, err := player.Leave(ctx)
					require.ErrorIs(t, err, ErrNotImplemented)
					_, err = player.Command(ctx, runtime.Command{Token: "t1", Name: "play_card"})
					require.ErrorIs(t, err, ErrNotImplemented)
				},
				expectedOK:          true,
				expectedLobby:       []string{"user-a"},
				expectedPlayerLobby: []string{"user-a"},
			}
		},
		"same_user_replaces_their_older_connection_even_when_full": func(t *testing.T) test {
			var older runtime.PlayerConnection
			return test{
				maxPlayers: 1,
				before:     func(t *testing.T, controller *Controller) { older = admit(t, controller, "user-a") },
				after: func(t *testing.T, controller *Controller, _ runtime.PlayerConnection) {
					require.Equal(t, runtime.EndReplaced, endedWith(t, older.Ended))
					// The replaced connection going away keeps the user in.
					older.Disconnect()
				},
				expectedOK:          true,
				expectedLobby:       []string{"user-a"},
				expectedPlayerLobby: []string{"user-a"},
			}
		},
		"leaving_takes_the_player_out_and_they_may_come_back": func(t *testing.T) test {
			return test{
				maxPlayers: 8,
				before:     func(t *testing.T, controller *Controller) { admit(t, controller, "user-b") },
				after: func(t *testing.T, controller *Controller, player runtime.PlayerConnection) {
					result, err := player.Leave(ctx)
					require.NoError(t, err)
					require.Equal(t, runtime.LeaveResult{Outcome: runtime.PlayerLeft}, result)
					require.Equal(t, []string{"user-b"}, controller.lobbySnapshot())
					admit(t, controller, "user-a")
					// The ended connection no longer speaks for the user.
					_, _ = player.Leave(ctx)
				},
				expectedOK:          true,
				expectedLobby:       []string{"user-b", "user-a"},
				expectedPlayerLobby: []string{"user-b", "user-a"},
				expectedEnd:         runtime.EndLeft,
			}
		},
		"disconnecting_takes_the_player_out": func(t *testing.T) test {
			return test{
				maxPlayers: 8,
				after: func(t *testing.T, controller *Controller, player runtime.PlayerConnection) {
					player.Disconnect()
					player.Disconnect()
				},
				expectedOK:          true,
				expectedLobby:       []string{},
				expectedPlayerLobby: []string{"user-a"},
				expectedEnd:         runtime.EndDisconnected,
			}
		},
		"commands_are_answered_as_not_running": func(t *testing.T) test {
			return test{
				maxPlayers: 8,
				after: func(t *testing.T, controller *Controller, player runtime.PlayerConnection) {
					result, err := player.Command(ctx, runtime.Command{Token: "t1", Name: "play_card"})
					require.NoError(t, err)
					require.Equal(t, runtime.CommandResult{Token: "t1", Outcome: runtime.CommandNotRunning}, result)
					require.NoError(t, player.Sample(ctx, runtime.Sample{Sequence: 1, Inputs: []byte(`{}`)}))
					require.NoError(t, player.Resync(ctx))
				},
				expectedOK:          true,
				expectedLobby:       []string{"user-a"},
				expectedPlayerLobby: []string{"user-a"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller := newTestController(t, tc.maxPlayers)
			admin := controller.AttachAdmin("creator")
			if tc.before != nil {
				tc.before(t, controller)
			}

			player, rejection, ok := controller.AdmitPlayer(ctx, "user-a")

			require.Equal(t, tc.expectedOK, ok)
			require.Equal(t, tc.expectedRejection, rejection)
			if ok && tc.after != nil {
				tc.after(t, controller, player)
			}
			require.Equal(t, tc.expectedLobby, controller.lobbySnapshot())
			require.Equal(t, tc.expectedLobby, latestLobby(t, admin.NextLobbyUpdate), "admins see every change")
			if !ok {
				return
			}
			require.Equal(t, tc.expectedPlayerLobby, latestLobby(t, player.NextLobbyUpdate))
			require.Equal(t, tc.expectedEnd, endedWith(t, player.Ended))
		})
	}
}
