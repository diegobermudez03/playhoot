package build

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
)

func TestInputsSHA256(t *testing.T) {
	revision := draftsources.Revision{
		Files:  map[string]draftsources.File{"server/game.ts": {SHA256: "a", Size: 1}, "client/Game.tsx": {SHA256: "b", Size: 2}},
		Assets: map[string]draftsources.AssetBinding{"cardBack": {SHA256: "c", Size: 3}},
	}
	same := draftsources.Revision{
		Files:  map[string]draftsources.File{"client/Game.tsx": {SHA256: "b", Size: 2}, "server/game.ts": {SHA256: "a", Size: 1}},
		Assets: map[string]draftsources.AssetBinding{"cardBack": {SHA256: "c", Size: 3}},
	}
	require.Equal(t, InputsSHA256(revision, "t1"), InputsSHA256(same, "t1"), "map order does not matter")
	require.NotEqual(t, InputsSHA256(revision, "t1"), InputsSHA256(revision, "t2"), "another toolchain builds anew")

	changed := draftsources.Revision{Files: revision.Files, Assets: map[string]draftsources.AssetBinding{"cardBack": {SHA256: "d", Size: 3}}}
	require.NotEqual(t, InputsSHA256(revision, "t1"), InputsSHA256(changed, "t1"), "assets are inputs")
}

func TestStatus(t *testing.T) {
	for _, status := range []Status{Succeeded, Failed, Superseded} {
		require.True(t, status.Terminal())
		require.True(t, status.Known())
	}
	require.False(t, Queued.Terminal())
	require.False(t, Running.Terminal())
	require.False(t, Status("PAUSED").Known())
}
