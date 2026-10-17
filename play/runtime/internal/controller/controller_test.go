package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestControllerFor(t *testing.T) {
	type test struct {
		// existingEpoch, when set, is the epoch of a controller already
		// registered for the Session.
		existingEpoch int64
		epoch         int64

		expectedOK bool
		// expectedReplaced means the existing controller was retired and a
		// new one returned.
		expectedReplaced bool
		// expectedCreations is how many controller creations the call under
		// test recorded.
		expectedCreations int
	}

	tests := map[string]func(t *testing.T) test{
		"no_controller_creates_one": func(t *testing.T) test {
			return test{epoch: 1, expectedOK: true, expectedCreations: 1}
		},
		"existing_controller_under_the_same_epoch_is_reused": func(t *testing.T) test {
			return test{existingEpoch: 1, epoch: 1, expectedOK: true}
		},
		"older_epoch_than_the_existing_controller_is_refused": func(t *testing.T) test {
			return test{existingEpoch: 2, epoch: 1}
		},
		"newer_epoch_retires_and_replaces_the_existing_controller": func(t *testing.T) test {
			return test{existingEpoch: 1, epoch: 2, expectedOK: true, expectedReplaced: true, expectedCreations: 1}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			ctx := context.Background()
			registry := NewRegistry()
			var existing *Controller
			var existingAdmin runtime.AdminConnection
			if tc.existingEpoch != 0 {
				existing, _ = registry.ControllerFor(ctx, "session-1", tc.existingEpoch)
				existingAdmin = existing.AttachAdmin("creator")
			}

			logCtx := logging.Start(ctx)
			controller, ok := registry.ControllerFor(logCtx, "session-1", tc.epoch)

			require.Equal(t, tc.expectedOK, ok)
			require.Equal(t, tc.expectedCreations, recordedCreations(t, logCtx))
			if !tc.expectedOK {
				require.Nil(t, controller)
				return
			}
			require.NotNil(t, controller)
			if existing == nil {
				return
			}
			if !tc.expectedReplaced {
				require.Same(t, existing, controller)
				return
			}
			require.NotSame(t, existing, controller)
			require.Equal(t, 0, existing.AdminCount())
			end, ended := existingAdmin.Ended(ctx)
			require.True(t, ended)
			require.Equal(t, runtime.ConnectionEnd{Reason: runtime.EndOwnerChanged}, end)
			again, _ := registry.ControllerFor(ctx, "session-1", tc.epoch)
			require.Same(t, controller, again)
		})
	}
}

// recordedCreations counts the controller creations ctx's request log holds.
func recordedCreations(t *testing.T, ctx context.Context) int {
	t.Helper()
	var output bytes.Buffer
	logging.FinishRequestLog(ctx, slog.New(slog.NewJSONHandler(&output, nil)), "test")
	var entry struct {
		Metrics []struct {
			Metric string `json:"metric"`
		} `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	creations := 0
	for _, measurement := range entry.Metrics {
		if measurement.Metric == "play_runtime_controllers_created_total" {
			creations++
		}
	}
	return creations
}
