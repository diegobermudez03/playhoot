package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// loadManifest is a ManifestLoader answering manifest.
func loadManifest(manifest Manifest) ManifestLoader {
	return func(context.Context) (Manifest, error) {
		return manifest, nil
	}
}

// newTestController returns a fresh Controller whose game allows maxPlayers.
func newTestController(t *testing.T, maxPlayers int) *Controller {
	t.Helper()
	controller, ok, err := NewRegistry().ControllerFor(context.Background(), "session-1", 1, loadManifest(Manifest{MinPlayers: 1, MaxPlayers: maxPlayers}))
	require.NoError(t, err)
	require.True(t, ok)
	return controller
}

func TestControllerFor(t *testing.T) {
	type test struct {
		// existingEpoch, when set, is the epoch of a controller already
		// registered for the Session.
		existingEpoch int64
		epoch         int64
		loadErr       error

		expectedOK  bool
		expectedErr bool
		// expectedReplaced means the existing controller was retired and a
		// new one returned.
		expectedReplaced bool
		// expectedCreations is how many controller creations the call under
		// test recorded, and expectedLoads how many manifests it loaded.
		expectedCreations int
		expectedLoads     int
	}

	manifest := Manifest{MinPlayers: 2, MaxPlayers: 6}

	tests := map[string]func(t *testing.T) test{
		"no_controller_creates_one_with_the_manifest": func(t *testing.T) test {
			return test{epoch: 1, expectedOK: true, expectedCreations: 1, expectedLoads: 1}
		},
		"existing_controller_under_the_same_epoch_is_reused_without_loading": func(t *testing.T) test {
			return test{existingEpoch: 1, epoch: 1, expectedOK: true}
		},
		"older_epoch_than_the_existing_controller_is_refused": func(t *testing.T) test {
			return test{existingEpoch: 2, epoch: 1}
		},
		"newer_epoch_retires_and_replaces_the_existing_controller": func(t *testing.T) test {
			return test{existingEpoch: 1, epoch: 2, expectedOK: true, expectedReplaced: true, expectedCreations: 1, expectedLoads: 1}
		},
		"loading_the_manifest_fails": func(t *testing.T) test {
			return test{epoch: 1, loadErr: errors.New("database down"), expectedErr: true, expectedLoads: 1}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			ctx := context.Background()
			registry := NewRegistry()
			var existing *Controller
			var existingAdmin runtime.AdminConnection
			var existingPlayer runtime.PlayerConnection
			if tc.existingEpoch != 0 {
				existing, _, _ = registry.ControllerFor(ctx, "session-1", tc.existingEpoch, loadManifest(manifest))
				existingAdmin = existing.AttachAdmin("creator")
				existingPlayer, _, _ = existing.AdmitPlayer(ctx, "player")
			}

			loads := 0
			load := func(context.Context) (Manifest, error) {
				loads++
				return manifest, tc.loadErr
			}
			logCtx := logging.Start(ctx)
			controller, ok, err := registry.ControllerFor(logCtx, "session-1", tc.epoch, load)

			require.Equal(t, tc.expectedLoads, loads)
			require.Equal(t, tc.expectedCreations, recordedCreations(t, logCtx))
			if tc.expectedErr {
				require.Error(t, err)
				require.Nil(t, controller)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedOK, ok)
			if !tc.expectedOK {
				require.Nil(t, controller)
				return
			}
			require.NotNil(t, controller)
			require.Equal(t, manifest, controller.manifest)
			if existing == nil {
				return
			}
			if !tc.expectedReplaced {
				require.Same(t, existing, controller)
				return
			}
			require.NotSame(t, existing, controller)
			require.Equal(t, 0, existing.AdminCount())
			require.Equal(t, 0, existing.PlayerCount())
			for _, ended := range []func(context.Context) (runtime.ConnectionEnd, bool){existingAdmin.Ended, existingPlayer.Ended} {
				end, ok := ended(ctx)
				require.True(t, ok)
				require.Equal(t, runtime.ConnectionEnd{Reason: runtime.EndOwnerChanged}, end)
			}
			again, _, _ := registry.ControllerFor(ctx, "session-1", tc.epoch, loadManifest(manifest))
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
