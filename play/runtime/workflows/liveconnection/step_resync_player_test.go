package liveconnection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestResync(t *testing.T) {
	type test struct {
		// asking returns the connection that asks for a resync, after
		// anything it does against the running game.
		asking func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection

		expectedViews []runtime.OwnView
	}

	ctx := context.Background()
	// committed moves P1's view to view, acknowledging their SAMPLEs.
	committed := func(t *testing.T, controller *sessionController, revision int64, view string) {
		work, _ := controller.takeBoundary()
		require.NoError(t, controller.commitBoundary(ctx, work, execution{
			game:  Game{ContextRevision: revision, State: []byte(`{}`)},
			views: map[string][]byte{"P1": []byte(view)},
		}))
	}

	tests := map[string]func(t *testing.T) test{
		"current_view_is_sent_whole_with_the_acknowledgement": func(t *testing.T) test {
			return test{
				asking: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection {
					require.NoError(t, players["user-a"].Sample(ctx, sample(3, `{"move":[1,0]}`)))
					committed(t, controller, 2, `{"x":2,"y":0}`)
					require.Len(t, drainViews(t, players["user-a"]), 1, "the patch to revision 2")
					return players["user-a"]
				},
				expectedViews: []runtime.OwnView{{
					PlayerView:                    runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 3, State: []byte(`{"x":2,"y":0}`)},
					ConsumedThroughSampleSequence: 3,
				}},
			}
		},
		"replaced_connection_gets_nothing": func(t *testing.T) test {
			return test{
				asking: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection {
					_, participant, _ := controller.reattachPlayer(ctx, "user-a")
					require.True(t, participant)
					return players["user-a"]
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{"x":1,"y":0}`)}, "user-a")
			asking := tc.asking(t, controller, players)

			require.NoError(t, asking.Resync(ctx))

			require.Equal(t, tc.expectedViews, drainViews(t, asking))
		})
	}
}

func TestPatchesFollowTheResyncBaseline(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{"x":1,"y":0}`)}, "user-a")
	ctx := context.Background()
	player := players["user-a"]
	require.NoError(t, player.Resync(ctx))
	require.Len(t, drainViews(t, player), 1)
	work, _ := controller.takeBoundary()

	require.NoError(t, controller.commitBoundary(ctx, work, execution{
		game:  Game{ContextRevision: 2, State: []byte(`{}`)},
		views: map[string][]byte{"P1": []byte(`{"x":5,"y":0}`)},
	}))

	require.Equal(t, []runtime.OwnView{{PlayerView: runtime.PlayerView{ParticipantID: "P1", Revision: 3, State: []byte(`{"x":5}`)}}}, drainViews(t, player))
}

func TestResyncBeforeTheGameRunsSendsNothing(t *testing.T) {
	controller := newTestController(t, 4)
	player, _, admitted := controller.joinLobby(context.Background(), "user-a")
	require.True(t, admitted)

	require.NoError(t, player.Resync(context.Background()))

	require.Empty(t, drainViews(t, player))
}
