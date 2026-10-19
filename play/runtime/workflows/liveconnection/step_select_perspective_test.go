package liveconnection

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestSelectPerspective(t *testing.T) {
	type test struct {
		// admin is the admin connection that selects, after anything it did
		// against its controller.
		admin         runtime.AdminConnection
		participantID string

		expectedOutcome runtime.SelectPerspectiveOutcome
		expectedViews   []runtime.WatchedView
	}

	ctx := context.Background()
	views := map[string][]byte{"P1": []byte(`{"x":1}`), "P2": []byte(`{"x":2}`)}
	running := func(t *testing.T, userUUIDs ...string) *sessionController {
		controller, _, _ := runningController(t, views, userUUIDs...)
		return controller
	}

	tests := map[string]func(t *testing.T) test{
		"selected_participant_is_sent_whole_without_acknowledgement": func(t *testing.T) test {
			return test{
				admin:           running(t, "user-a", "user-b").registerAdmin("creator"),
				participantID:   "P2",
				expectedOutcome: runtime.PerspectiveSelected,
				expectedViews:   []runtime.WatchedView{{PlayerView: runtime.PlayerView{ParticipantID: "P2", Full: true, Revision: 1, State: []byte(`{"x":2}`)}}},
			}
		},
		"each_selection_is_a_fresh_baseline": func(t *testing.T) test {
			admin := running(t, "user-a", "user-b").registerAdmin("creator")
			selected, err := admin.SelectPerspective(ctx, "P1")
			require.NoError(t, err)
			require.Equal(t, runtime.PerspectiveSelected, selected.Outcome)
			require.Len(t, drainWatched(t, admin), 1)
			return test{
				admin:           admin,
				participantID:   "P2",
				expectedOutcome: runtime.PerspectiveSelected,
				expectedViews:   []runtime.WatchedView{{PlayerView: runtime.PlayerView{ParticipantID: "P2", Full: true, Revision: 2, State: []byte(`{"x":2}`)}}},
			}
		},
		"unknown_participant": func(t *testing.T) test {
			return test{
				admin:           running(t, "user-a").registerAdmin("creator"),
				participantID:   "P9",
				expectedOutcome: runtime.PerspectiveUnknownParticipant,
			}
		},
		"creator_who_plays_watches_nobody": func(t *testing.T) test {
			return test{
				admin:           running(t, "creator", "user-b").registerAdmin("creator"),
				participantID:   "P2",
				expectedOutcome: runtime.PerspectiveCreatorPlaying,
			}
		},
		"creator_who_left_the_lobby_watches_nobody": func(t *testing.T) test {
			controller := newTestController(t, 4)
			creator, _, admitted := controller.joinLobby(ctx, "creator")
			require.True(t, admitted)
			creator.Disconnect()
			_, _, admitted = controller.joinLobby(ctx, "user-a")
			require.True(t, admitted)
			participants, _, ok := controller.beginStart()
			require.True(t, ok)
			controller.completeStart(participants, Game{ContextRevision: 1, State: []byte(`{}`)}, runtime.GameFrontend{}, views, nil)
			return test{
				admin:           controller.registerAdmin("creator"),
				participantID:   "P1",
				expectedOutcome: runtime.PerspectiveCreatorPlaying,
			}
		},
		"lobby_is_not_running": func(t *testing.T) test {
			return test{
				admin:           newTestController(t, 4).registerAdmin("creator"),
				participantID:   "P1",
				expectedOutcome: runtime.PerspectiveNotRunning,
			}
		},
		"disconnected_admin_selects_nothing": func(t *testing.T) test {
			admin := running(t, "user-a").registerAdmin("creator")
			admin.Disconnect()
			return test{
				admin:           admin,
				participantID:   "P1",
				expectedOutcome: runtime.PerspectiveNotRunning,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			result, err := tc.admin.SelectPerspective(ctx, tc.participantID)

			require.NoError(t, err)
			require.Equal(t, runtime.SelectPerspectiveResult{Outcome: tc.expectedOutcome}, result)
			require.Equal(t, tc.expectedViews, drainWatched(t, tc.admin))
		})
	}
}

func TestWatchedViews(t *testing.T) {
	type test struct {
		executed execution
		// sampled means P1 sends a SAMPLE before the boundary.
		sampled bool

		expectedAdminViews  []runtime.WatchedView
		expectedPlayerViews []runtime.OwnView
	}

	ctx := context.Background()
	committed := func(views map[string][]byte) execution {
		return execution{game: Game{ContextRevision: 2, State: []byte(`{}`)}, views: views}
	}

	tests := map[string]func(t *testing.T) test{
		"admin_gets_the_patch_and_events_without_errors_or_acknowledgement": func(t *testing.T) test {
			executed := committed(map[string][]byte{"P1": []byte(`{"x":5,"y":0}`), "P2": []byte(`{"x":2,"y":0}`)})
			executed.errors = []gameError{{participantID: "P1", value: []byte(`"blocked"`)}}
			executed.events = []gameEvent{{value: []byte(`"bell"`)}}
			return test{
				executed: executed,
				sampled:  true,
				expectedAdminViews: []runtime.WatchedView{{
					PlayerView: runtime.PlayerView{ParticipantID: "P1", Revision: 2, State: []byte(`{"x":5}`)},
					Events:     [][]byte{[]byte(`"bell"`)},
				}},
				expectedPlayerViews: []runtime.OwnView{{
					PlayerView:                    runtime.PlayerView{ParticipantID: "P1", Revision: 2, State: []byte(`{"x":5}`)},
					ConsumedThroughSampleSequence: 3,
					Errors:                        [][]byte{[]byte(`"blocked"`)},
					Events:                        [][]byte{[]byte(`"bell"`)},
				}},
			}
		},
		"admin_gets_nothing_from_an_acknowledgement_or_an_error_alone": func(t *testing.T) test {
			executed := committed(map[string][]byte{"P1": []byte(`{"x":1,"y":0}`), "P2": []byte(`{"x":2,"y":0}`)})
			executed.errors = []gameError{{participantID: "P1", value: []byte(`"blocked"`)}}
			return test{
				executed: executed,
				sampled:  true,
				expectedPlayerViews: []runtime.OwnView{{
					PlayerView:                    runtime.PlayerView{ParticipantID: "P1", Revision: 1, State: []byte(`{}`)},
					ConsumedThroughSampleSequence: 3,
					Errors:                        [][]byte{[]byte(`"blocked"`)},
				}},
			}
		},
		"events_for_someone_else_do_not_reach_the_admin": func(t *testing.T) test {
			executed := committed(map[string][]byte{"P1": []byte(`{"x":1,"y":0}`), "P2": []byte(`{"x":2,"y":0}`)})
			executed.events = []gameEvent{{participantIDs: []string{"P2"}, value: []byte(`"secret"`)}}
			return test{executed: executed}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{"x":1,"y":0}`), "P2": []byte(`{"x":2,"y":0}`)}, "user-a", "user-b")
			admin := controller.registerAdmin("creator")
			_, err := admin.SelectPerspective(ctx, "P1")
			require.NoError(t, err)
			require.Len(t, drainWatched(t, admin), 1, "the FULL view of the selection")
			if tc.sampled {
				require.NoError(t, players["user-a"].Sample(ctx, sample(3, `{"move":[1,0]}`)))
			}
			work, _ := controller.takeBoundary()

			require.NoError(t, controller.commitBoundary(ctx, work, tc.executed))

			require.Equal(t, tc.expectedAdminViews, drainWatched(t, admin))
			require.Equal(t, tc.expectedPlayerViews, drainViews(t, players["user-a"]))
		})
	}
}

func TestAdminResync(t *testing.T) {
	type test struct {
		selects string

		expectedViews []runtime.WatchedView
	}

	ctx := context.Background()
	tests := map[string]func(t *testing.T) test{
		"watched_view_is_sent_whole_again": func(t *testing.T) test {
			return test{
				selects:       "P1",
				expectedViews: []runtime.WatchedView{{PlayerView: runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 2, State: []byte(`{"x":1}`)}}},
			}
		},
		"nothing_without_a_selection": func(t *testing.T) test {
			return test{}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, _, _ := runningController(t, map[string][]byte{"P1": []byte(`{"x":1}`)}, "user-a")
			admin := controller.registerAdmin("creator")
			if tc.selects != "" {
				_, err := admin.SelectPerspective(ctx, tc.selects)
				require.NoError(t, err)
				require.Len(t, drainWatched(t, admin), 1)
			}

			require.NoError(t, admin.Resync(ctx))

			require.Equal(t, tc.expectedViews, drainWatched(t, admin))
		})
	}
}

// drainWatched returns every view admin has ready, in order.
func drainWatched(t *testing.T, admin runtime.AdminConnection) []runtime.WatchedView {
	t.Helper()
	var views []runtime.WatchedView
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		view, ok := admin.NextView(ctx)
		cancel()
		if !ok {
			return views
		}
		if !json.Valid(view.State) {
			t.Fatalf("view state is not JSON: %s", view.State)
		}
		views = append(views, view)
	}
}
