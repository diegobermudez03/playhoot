package session

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
)

// ConfigPath is where a project keeps its configuration values
// (playhoot.config.v1); game.project.json's "config" declares them.
const ConfigPath = draftsources.ConfigFile

// ProjectPath is the project's manifest, game.project.json.
const ProjectPath = draftsources.ProjectFile

// DeclaredConfig is the "config" object content, a game.project.json,
// declares as written, or nil when it declares none.
func DeclaredConfig(content string) json.RawMessage {
	return draftsources.DeclaredConfig(content)
}
