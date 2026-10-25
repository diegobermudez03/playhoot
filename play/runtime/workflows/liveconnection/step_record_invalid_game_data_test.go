package liveconnection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestInvalidGameDataIsRecordedBeforeItIsRefused(t *testing.T) {
	type test struct {
		// send sends the invalid data from user-a's connection and returns
		// its error.
		send     func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) error
		storeErr error

		expectedErrorIs error
		expected        []sessionpkg.GameFault
	}

	ctx := context.Background()
	invalidSample := func(detail string) []sessionpkg.GameFault {
		return []sessionpkg.GameFault{{
			UserUUID: "user-a", ParticipantID: "P1", Source: sessionpkg.GameFaultFromServer,
			Kind: sessionpkg.GameFaultInvalidSample, Detail: []byte(detail),
		}}
	}
	invalidCommand := func(detail string) []sessionpkg.GameFault {
		return []sessionpkg.GameFault{{
			UserUUID: "user-a", ParticipantID: "P1", Source: sessionpkg.GameFaultFromServer,
			Kind: sessionpkg.GameFaultInvalidCommand, Detail: []byte(detail),
		}}
	}

	tests := map[string]func(t *testing.T) test{
		"an_undeclared_input": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) error {
					return players["user-a"].Sample(ctx, sample(1, `{"jump":true}`))
				},
				expectedErrorIs: runtime.ErrInvalidSample,
				expected:        invalidSample(`{"reason":"invalid input value: jump is not a declared input"}`),
			}
		},
		"an_input_value_of_the_wrong_type": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) error {
					return players["user-a"].Sample(ctx, sample(1, `{"move":"left"}`))
				},
				expectedErrorIs: runtime.ErrInvalidSample,
				expected:        invalidSample(`{"reason":"invalid input value: move must be two numbers"}`),
			}
		},
		"an_undeclared_command": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) error {
					_, err := players["user-a"].Command(ctx, runtime.Command{Token: "t1", Name: "fly"})
					return err
				},
				expectedErrorIs: runtime.ErrInvalidCommand,
				expected:        invalidCommand(`{"command":"fly","reason":"undeclared command: fly"}`),
			}
		},
		"a_payload_out_of_its_declared_range": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) error {
					_, err := players["user-a"].Command(ctx, runtime.Command{Token: "t1", Name: "move_to", Payload: []byte(`{"x":11}`)})
					return err
				},
				expectedErrorIs: runtime.ErrInvalidCommand,
				expected:        invalidCommand(`{"command":"move_to","reason":"invalid input value: x is out of range"}`),
			}
		},
		"a_storing_failure_refuses_it_all_the_same": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) error {
					return players["user-a"].Sample(ctx, sample(1, `{"jump":true}`))
				},
				storeErr:        errors.New("database down"),
				expectedErrorIs: runtime.ErrInvalidSample,
				expected:        invalidSample(`{"reason":"invalid input value: jump is not a declared input"}`),
			}
		},
		"a_replaced_connection_s_command_is_not_running_and_not_recorded": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) error {
					_, participant, _ := controller.reattachPlayer(ctx, runtime.Player{UserUUID: "user-a"}, nil)
					require.True(t, participant)
					result, err := players["user-a"].Command(ctx, runtime.Command{Token: "t1", Name: "fly"})
					require.Equal(t, runtime.CommandNotRunning, result.Outcome)
					return err
				},
			}
		},
		"a_replaced_connection_s_sample_is_dropped_and_not_recorded": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) error {
					_, participant, _ := controller.reattachPlayer(ctx, runtime.Player{UserUUID: "user-a"}, nil)
					require.True(t, participant)
					return players["user-a"].Sample(ctx, sample(1, `{"jump":true}`))
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
			faults := &recordedFaults{err: tc.storeErr}
			controller.deps.faults = faults

			err := tc.send(t, controller, players)

			if tc.expectedErrorIs != nil {
				require.ErrorIs(t, err, tc.expectedErrorIs)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expected, faults.faults)
			for _, sessionUUID := range faults.sessions {
				require.Equal(t, controller.sessionUUID, sessionUUID)
			}
			require.Empty(t, timelineCommands(controller), "invalid data never reaches the game")
			boundary, ok := controller.takeBoundary()
			require.True(t, ok)
			require.Empty(t, withoutStreams(boundary.operations))
		})
	}
}

// blockingFaults is a gameFaultRecorder that holds every record until
// released.
type blockingFaults struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingFaults) RecordGameFault(context.Context, string, sessionpkg.GameFault) (sessionpkg.RecordGameFaultOutcome, error) {
	b.started <- struct{}{}
	<-b.release
	return sessionpkg.GameFaultRecorded, nil
}

func TestRecordingInvalidGameDataDoesNotHoldTheGame(t *testing.T) {
	ctx := context.Background()
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
	faults := &blockingFaults{started: make(chan struct{}, 1), release: make(chan struct{})}
	controller.deps.faults = faults
	refused := make(chan error, 1)
	go func() { refused <- players["user-a"].Sample(ctx, sample(1, `{"jump":true}`)) }()
	<-faults.started

	// The other player keeps playing while the record is stored.
	require.NoError(t, players["user-b"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
	select {
	case <-refused:
		t.Fatal("refused before it was recorded")
	default:
	}
	close(faults.release)

	select {
	case err := <-refused:
		require.ErrorIs(t, err, runtime.ErrInvalidSample)
	case <-time.After(time.Second):
		t.Fatal("never refused")
	}
}

func TestInvalidGameDataDetailFitsItsRecord(t *testing.T) {
	type test struct {
		detail []byte

		expected map[string]string
	}

	// Every byte of these needs escaping in JSON.
	escaped := strings.Repeat("<", 1000)
	tests := map[string]func(t *testing.T) test{
		"sample": func(t *testing.T) test {
			return test{
				detail:   invalidSampleDetail(errors.New("invalid input value: jump is not a declared input")),
				expected: map[string]string{"reason": "invalid input value: jump is not a declared input"},
			}
		},
		"command": func(t *testing.T) test {
			return test{
				detail:   invalidCommandDetail("fly", errors.New("undeclared command: fly")),
				expected: map[string]string{"command": "fly", "reason": "undeclared command: fly"},
			}
		},
		"long_values_are_cut_whole_runes": func(t *testing.T) test {
			return test{
				detail: invalidCommandDetail(strings.Repeat("é", 100), errors.New(escaped)),
				expected: map[string]string{
					"command": strings.Repeat("é", maxInvalidCommandBytes/2),
					"reason":  escaped[:maxInvalidReasonBytes],
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			var decoded map[string]string
			require.NoError(t, json.Unmarshal(tc.detail, &decoded))
			require.Equal(t, tc.expected, decoded)
			require.LessOrEqual(t, len(tc.detail), sessionpkg.MaxGameFaultDetailBytes)
		})
	}
}
