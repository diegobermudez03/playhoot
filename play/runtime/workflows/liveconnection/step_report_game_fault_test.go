package liveconnection

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// recordedFaults is a gameFaultRecorder keeping what it was asked to record.
type recordedFaults struct {
	mu       sync.Mutex
	sessions []string
	faults   []sessionpkg.GameFault
	err      error
}

func (r *recordedFaults) RecordGameFault(_ context.Context, sessionUUID string, fault sessionpkg.GameFault) (sessionpkg.RecordGameFaultOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = append(r.sessions, sessionUUID)
	r.faults = append(r.faults, fault)
	if r.err != nil {
		return "", r.err
	}
	return sessionpkg.GameFaultRecorded, nil
}

func TestReportGameFault(t *testing.T) {
	type test struct {
		// reporting returns the connection that reports, after anything it
		// does to the controller.
		reporting func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection
		fault     runtime.GameFault
		storeErr  error

		expected []sessionpkg.GameFault
	}

	ctx := context.Background()
	abuse := runtime.GameFault{Kind: runtime.GameFaultBridgeAbuse, Detail: []byte(`{"refused":{"sample":900}}`)}

	tests := map[string]func(t *testing.T) test{
		"a_participant_of_the_running_game_is_recorded_with_who_they_are": func(t *testing.T) test {
			return test{
				reporting: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection {
					return players["user-b"]
				},
				fault: abuse,
				expected: []sessionpkg.GameFault{{
					UserUUID: "user-b", ParticipantID: "P2", Source: sessionpkg.GameFaultFromClient, Kind: sessionpkg.GameFaultBridgeAbuse, Detail: abuse.Detail,
				}},
			}
		},
		"every_kind_maps_to_its_durable_kind": func(t *testing.T) test {
			return test{
				reporting: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection {
					return players["user-a"]
				},
				fault: runtime.GameFault{Kind: runtime.GameFaultContainerViolation, Detail: []byte(`{}`)},
				expected: []sessionpkg.GameFault{{
					UserUUID: "user-a", ParticipantID: "P1", Source: sessionpkg.GameFaultFromClient, Kind: sessionpkg.GameFaultContainerViolation, Detail: []byte(`{}`),
				}},
			}
		},
		"an_unknown_kind_is_dropped": func(t *testing.T) test {
			return test{
				reporting: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection {
					return players["user-a"]
				},
				fault: runtime.GameFault{Kind: "cheating", Detail: []byte(`{}`)},
			}
		},
		"a_replaced_connection_is_dropped": func(t *testing.T) test {
			return test{
				reporting: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection {
					_, participant, _ := controller.reattachPlayer(ctx, runtime.Player{UserUUID: "user-a"}, nil)
					require.True(t, participant)
					return players["user-a"]
				},
				fault: abuse,
			}
		},
		"a_storing_failure_never_fails_the_connection": func(t *testing.T) test {
			return test{
				reporting: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection {
					return players["user-a"]
				},
				fault:    abuse,
				storeErr: errors.New("database down"),
				expected: []sessionpkg.GameFault{{
					UserUUID: "user-a", ParticipantID: "P1", Source: sessionpkg.GameFaultFromClient, Kind: sessionpkg.GameFaultBridgeAbuse, Detail: abuse.Detail,
				}},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
			faults := &recordedFaults{err: tc.storeErr}
			controller.deps.faults = faults
			reporting := tc.reporting(t, controller, players)

			require.NoError(t, reporting.ReportGameFault(ctx, tc.fault))

			require.Equal(t, tc.expected, faults.faults)
			for _, sessionUUID := range faults.sessions {
				require.Equal(t, controller.sessionUUID, sessionUUID)
			}
		})
	}
}

func TestReportGameFaultBeforeTheGameRunsIsDropped(t *testing.T) {
	controller := newTestController(t, 4)
	faults := &recordedFaults{}
	controller.deps.faults = faults
	player, _, admitted := controller.joinLobby(context.Background(), runtime.Player{UserUUID: "user-a"})
	require.True(t, admitted)

	require.NoError(t, player.ReportGameFault(context.Background(), runtime.GameFault{Kind: runtime.GameFaultReadyTimeout, Detail: []byte(`{}`)}))

	require.Empty(t, faults.faults)
}
