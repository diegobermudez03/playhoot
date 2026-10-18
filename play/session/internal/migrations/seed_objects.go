package migrations

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
)

// Object storage prefixes of game version content, which is content-addressed
// below them.
const (
	backendScriptPrefix   = "game-versions/backend-scripts"
	frontendPackagePrefix = "game-versions/frontend-packages"
)

// seedBackendScript is the playable seed version's backend script. It is test
// content, not a real game.
const seedBackendScript = `function start(participants, context) {
	return { state: { round: 1, positions: Object.fromEntries(participants.map((p) => [p, [0, 0]])) } };
}

function samples(state, transitions, context) {
	const positions = { ...state.positions };
	for (const transition of transitions) {
		const [x, y] = positions[transition.participant];
		positions[transition.participant] = [x + transition.inputs.move[0], y + transition.inputs.move[1]];
	}
	return { state: { round: state.round, positions } };
}

function project(state, participant, context) {
	const others = {};
	for (const id of Object.keys(state.positions)) {
		if (id !== participant) {
			others[id] = state.positions[id];
		}
	}
	return { round: state.round, me: state.positions[participant], others };
}
`

// seedFrontendPackage is the playable seed version's frontend package: one
// self-contained HTML file. It is test content, not a real game.
const seedFrontendPackage = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>Seed game</title></head>
<body>
<p id="view">Waiting for the game...</p>
</body>
</html>
`

var (
	seedBackendScriptLocator   = objectstore.LocatorFor(backendScriptPrefix, []byte(seedBackendScript))
	seedFrontendPackageLocator = objectstore.LocatorFor(frontendPackagePrefix, []byte(seedFrontendPackage))
)

// SeedObjects stores the content the seed game versions reference. Keys are
// content-addressed, so running it again changes nothing.
func SeedObjects(ctx context.Context, store objectstore.Store) error {
	objects := []struct {
		prefix      string
		content     string
		contentType string
	}{
		{prefix: backendScriptPrefix, content: seedBackendScript, contentType: "text/javascript"},
		{prefix: backendScriptPrefix, content: seedSpaceDriftScript, contentType: "text/javascript"},
		{prefix: frontendPackagePrefix, content: seedFrontendPackage, contentType: "text/html"},
	}
	for _, object := range objects {
		if _, err := objectstore.PutContent(ctx, store, object.prefix, []byte(object.content), object.contentType); err != nil {
			return fmt.Errorf("storing seed object under %s: %w", object.prefix, err)
		}
	}
	return nil
}
