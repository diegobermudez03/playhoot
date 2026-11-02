package gamebuild

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
)

func TestUnconfigured(t *testing.T) {
	builder := NewNode("")
	_, err := builder.Version(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
	_, err = builder.Build(context.Background(), Input{Dir: t.TempDir()})
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestNodeVersion(t *testing.T) {
	root := t.TempDir()
	for _, pkg := range toolchainPackages {
		dir := filepath.Join(root, "packages", pkg, "src")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.ts"), []byte(pkg), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.test.ts"), []byte("test"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "package-lock.json"), []byte("{}"), 0o600))

	first, err := (&Node{root: root}).Version(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "packages", "game-core", "src", "index.test.ts"), []byte("changed"), 0o600))
	same, err := (&Node{root: root}).Version(context.Background())
	require.NoError(t, err)
	require.Equal(t, first, same, "tests are not part of the toolchain")

	require.NoError(t, os.WriteFile(filepath.Join(root, "packages", "game-core", "src", "index.ts"), []byte("changed"), 0o600))
	changed, err := (&Node{root: root}).Version(context.Background())
	require.NoError(t, err)
	require.NotEqual(t, first, changed)

	_, err = (&Node{root: filepath.Join(root, "missing")}).Version(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
}

// TestNodeBuildsAProject runs the real toolchain when GAME_TOOLCHAIN_TEST_DIR
// names a frontend repository checkout with its dependencies installed.
func TestNodeBuildsAProject(t *testing.T) {
	root := os.Getenv("GAME_TOOLCHAIN_TEST_DIR")
	if root == "" {
		t.Skip("GAME_TOOLCHAIN_TEST_DIR not set; the Node toolchain is not exercised")
	}
	project := t.TempDir()
	write := func(path string, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(project, path)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(project, path), []byte(content), 0o600))
	}
	write("game.project.json", `{"format":"playhoot.project.v1","name":"Probe","aspectRatio":"1:1"}`)
	write("server/game.ts", "import { defineGame } from '@playhoot/game-server';\nexport default defineGame({ players: { min: 1, max: 2 }, setup: () => ({ n: 0 }), view: (state) => state });\n")
	write("client/Game.tsx", "export default function Game() { return null; }\n")

	builder := NewNode(root)
	output, err := builder.Build(context.Background(), Input{Dir: project})
	require.NoError(t, err)
	require.True(t, output.OK, output.Diagnostics)
	require.Equal(t, "1:1", output.AspectRatio)
	require.Contains(t, string(output.Server), "function start(")
	require.Contains(t, string(output.Client), "<div id=\"root\"></div>")
	require.JSONEq(t, `{"schema_version":1,"players":{"min":1,"max":2},"inputs":{},"commands":{},"entrypoints":[]}`, string(output.Manifest))

	write("server/game.ts", "import { defineGame } from '@playhoot/game-server';\nexport default defineGame({ players: { min: 1, max: 2 }, setup: () => ({ n: 0 }), view: (state) => state.missing.field });\n")
	failed, err := builder.Build(context.Background(), Input{Dir: project})
	require.NoError(t, err)
	require.False(t, failed.OK)
	require.Equal(t, build.StageTypes, failed.Diagnostics[0].Stage)
	require.Equal(t, "server/game.ts", failed.Diagnostics[0].File)
	require.Equal(t, 2, failed.Diagnostics[0].Line)
}
