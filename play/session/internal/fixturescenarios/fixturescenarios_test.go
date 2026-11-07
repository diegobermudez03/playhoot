package fixturescenarios

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	"github.com/diegobermudez03/playhoot/play/runtime/usecases/simulategame"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gamebrief"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/manifest"
)

func TestMain(m *testing.M) {
	if sandbox.RunAsWorkerIfRequested() {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fixturesDir is the frontend checkout's SDK fixtures: GAME_TOOLCHAIN_DIR's
// (the checkout the builds use), or the sibling checkout.
func fixturesDir() string {
	root := os.Getenv("GAME_TOOLCHAIN_DIR")
	if root == "" {
		root = filepath.Join("..", "..", "..", "..", "..", "playhoot_frontend")
	}
	return filepath.Join(root, "fixtures", "sdk-games")
}

func TestFixtureScenariosHold(t *testing.T) {
	root := fixturesDir()
	games, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("no frontend checkout at %s: %v", root, err)
	}
	host := sandbox.NewHost(sandbox.HostLimits{})
	t.Cleanup(func() { _ = host.Close() })
	service := simulategame.New(host, discard{})

	for _, game := range games {
		dir := filepath.Join(root, game.Name())
		scenarios, err := filepath.Glob(filepath.Join(dir, "scenarios", "*.json"))
		require.NoError(t, err)
		if len(scenarios) == 0 {
			continue
		}
		t.Run(game.Name(), func(t *testing.T) {
			brief, err := os.ReadFile(filepath.Join(dir, "brief.json"))
			require.NoError(t, err, "every fixture keeps its brief")
			_, problems := gamebrief.Parse(brief)
			require.Empty(t, problems, "the platform accepts the fixture's brief")

			script, err := os.ReadFile(filepath.Join(dir, "dist", "server.js"))
			if err != nil {
				t.Skipf("%s is not built (npm run game -- build fixtures/sdk-games/%s in the frontend checkout)", game.Name(), game.Name())
			}
			rawManifest, err := os.ReadFile(filepath.Join(dir, "dist", "manifest.json"))
			require.NoError(t, err)
			parsed, err := manifest.Parse(rawManifest)
			require.NoError(t, err, "the built manifest is valid for the platform")

			for _, path := range scenarios {
				raw, err := os.ReadFile(path)
				require.NoError(t, err)
				scenario, diagnostics := runtime.DecodeScenario(raw)
				require.Empty(t, diagnostics, path)
				require.NotEmpty(t, scenario.Expect, "%s has expectations", path)

				result, err := service.Simulate(context.Background(), "fixtures", script, string(scriptcheck.ProfileSDKV1), parsed.Runtime(), scenario)

				require.NoError(t, err)
				require.Equal(t, runtime.SimulationRan, result.Outcome, "%s: %v", path, result.Diagnostics)
				for _, checked := range result.Expectations {
					require.True(t, checked.Passed, "%s expect[%d] %s %s: actual %s %s", path, checked.Index, checked.Expectation.Path, checked.Expectation.Op, checked.Actual, checked.Problem)
				}
			}
		})
	}
}

// discard keeps no result: the test reads only the answers.
type discard struct{}

func (discard) KeepSimulationResult(ctx context.Context, versionUUID string, resultUUID string, result json.RawMessage, expiresAt time.Time) error {
	return nil
}

func (discard) FindSimulationResult(ctx context.Context, resultUUID string, versionUUID string) (json.RawMessage, bool, error) {
	return nil, false, nil
}
