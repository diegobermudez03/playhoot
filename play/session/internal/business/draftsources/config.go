package draftsources

import (
	"bytes"
	"encoding/json"
)

// ConfigFile holds a project's configuration values (playhoot.config.v1):
// what a template exposes to be changed as data instead of code. Its
// declaration is game.project.json's "config"; the build checks the values
// against it, so here only the document's shape is checked, which keeps it
// readable for inspect_template.
const ConfigFile = "config.json"

// ConfigFormat is the format a config.json declares.
const ConfigFormat = "playhoot.config.v1"

// CodeInvalidConfig refuses a config.json that is not a playhoot.config.v1
// document.
const CodeInvalidConfig = "invalid_config"

// checkConfig is the message refusing content as a config.json, or "".
func checkConfig(content string) string {
	var document struct {
		Format string          `json:"format"`
		Values json.RawMessage `json:"values"`
	}
	trimmed := bytes.TrimSpace([]byte(content))
	if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &document) != nil ||
		document.Format != ConfigFormat || !bytes.HasPrefix(bytes.TrimSpace(document.Values), []byte("{")) {
		return `config.json is {"format": "` + ConfigFormat + `", "values": {...}}`
	}
	return ""
}

// DeclaredConfig is the "config" object projectFile, a game.project.json,
// declares as written, or nil when it declares none or is not JSON.
func DeclaredConfig(projectFile string) json.RawMessage {
	var project struct {
		Config json.RawMessage `json:"config"`
	}
	if json.Unmarshal([]byte(projectFile), &project) != nil {
		return nil
	}
	declared := bytes.TrimSpace(project.Config)
	if !bytes.HasPrefix(declared, []byte("{")) {
		return nil
	}
	return declared
}
