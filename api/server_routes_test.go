package api

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

const buildServerEnv = "PLAYHOOT_TEST_BUILD_SERVER"

// TestNewServerDeclaresEveryRoute builds the real server with every route
// group, so a metric outgrowing its series bound (routes times statuses)
// fails here instead of at startup. It runs in its own process: metrics are
// declared once per process, and other tests declare them over test routes.
func TestNewServerDeclaresEveryRoute(t *testing.T) {
	if os.Getenv(buildServerEnv) == "1" {
		NewServer(nil, nil, nil, nil, nil, nil, Logins{}, nil, nil, nil, nil, nil, Games{}, AI{}, Config{})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestNewServerDeclaresEveryRoute$")
	cmd.Env = append(os.Environ(), buildServerEnv+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	require.True(t, aiRoute("POST /mcp"))
	require.True(t, aiRoute("GET /.well-known/oauth-authorization-server"))
	require.True(t, aiRoute("GET /ai/v1/{token}"))
	require.False(t, aiRoute("GET /v1/games"))
}
