package draftsources

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
)

// ProjectFile is the project's manifest, which also declares its assets.
const ProjectFile = "game.project.json"

// Diagnostic codes of an invalid asset declaration.
const (
	CodeInvalidProjectFile = "invalid_project_file"
	CodeInvalidAsset       = "invalid_asset_declaration"
)

// maxAssetDescriptionLength bounds what a declaration may say about an asset.
const maxAssetDescriptionLength = 300

// AssetDeclaration is one asset the project says it uses: its type, what to
// upload for it, and whether the game works without it.
type AssetDeclaration struct {
	Type        gameversion.AssetType
	Description string
	Optional    bool
}

// AssetBinding is the uploaded content a revision uses for one declared key.
type AssetBinding struct {
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	MIMEType string `json:"mime_type"`
}

// Revision is everything a revision pins: its source files and the asset
// content bound to each key.
type Revision struct {
	Files  map[string]File
	Assets map[string]AssetBinding
}

// SameRevision reports whether a and b hold the same files and the same
// asset content under the same keys.
func SameRevision(a Revision, b Revision) bool {
	if !SameFiles(a.Files, b.Files) || len(a.Assets) != len(b.Assets) {
		return false
	}
	for key, binding := range a.Assets {
		other, found := b.Assets[key]
		if !found || other.SHA256 != binding.SHA256 {
			return false
		}
	}
	return true
}

// declaredAsset is an asset declaration as game.project.json writes it.
type declaredAsset struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Optional    bool   `json:"optional"`
}

// DeclaredAssets reads the assets game.project.json declares. An absent
// "assets" declares none. Diagnostics name what makes the file or a
// declaration invalid; then the result is nil.
func DeclaredAssets(projectFile string) (map[string]AssetDeclaration, []Diagnostic) {
	var project struct {
		Assets map[string]json.RawMessage `json:"assets"`
	}
	if err := json.Unmarshal([]byte(projectFile), &project); err != nil {
		return nil, []Diagnostic{{Path: ProjectFile, Code: CodeInvalidProjectFile, Message: fmt.Sprintf("%s is not a JSON object: %s", ProjectFile, err)}}
	}
	keys := make([]string, 0, len(project.Assets))
	for key := range project.Assets {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	declared := make(map[string]AssetDeclaration, len(project.Assets))
	var diagnostics []Diagnostic
	refuse := func(key, message string) {
		diagnostics = append(diagnostics, Diagnostic{Path: ProjectFile, Code: CodeInvalidAsset, Message: fmt.Sprintf("asset %q: %s", key, message)})
	}
	for _, key := range keys {
		if !gameversion.ValidAssetKey(key) {
			refuse(key, "keys start with a letter and hold up to 64 letters, digits or underscores")
			continue
		}
		var asset declaredAsset
		if err := json.Unmarshal(project.Assets[key], &asset); err != nil {
			refuse(key, `each asset is an object {"type", "description", "optional"}`)
			continue
		}
		assetType := gameversion.AssetType(asset.Type)
		if !gameversion.ValidAssetType(assetType) {
			refuse(key, `"type" is image, audio or video`)
			continue
		}
		description := strings.TrimSpace(asset.Description)
		if description == "" || len(description) > maxAssetDescriptionLength {
			refuse(key, fmt.Sprintf(`"description" says what to upload, in 1 to %d characters`, maxAssetDescriptionLength))
			continue
		}
		declared[key] = AssetDeclaration{Type: assetType, Description: description, Optional: asset.Optional}
	}
	if len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return declared, nil
}

// AssetStatus is where a revision stands with one asset key.
type AssetStatus string

const (
	AssetMissing    AssetStatus = "missing"
	AssetReady      AssetStatus = "ready"
	AssetUndeclared AssetStatus = "undeclared"
)

// AssetState is one key of a revision: its declaration, when declared, and
// its bound content, when any.
type AssetState struct {
	Key         string
	Status      AssetStatus
	Declaration *AssetDeclaration
	Binding     *AssetBinding
}

// AssetStates lists every declared key and every bound but undeclared key,
// sorted by key.
func AssetStates(declared map[string]AssetDeclaration, bindings map[string]AssetBinding) []AssetState {
	keys := make([]string, 0, len(declared)+len(bindings))
	for key := range declared {
		keys = append(keys, key)
	}
	for key := range bindings {
		if _, found := declared[key]; !found {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	states := make([]AssetState, len(keys))
	for i, key := range keys {
		state := AssetState{Key: key}
		if declaration, found := declared[key]; found {
			state.Declaration = &declaration
		}
		if binding, found := bindings[key]; found {
			state.Binding = &binding
		}
		switch {
		case state.Declaration == nil:
			state.Status = AssetUndeclared
		case state.Binding == nil:
			state.Status = AssetMissing
		default:
			state.Status = AssetReady
		}
		states[i] = state
	}
	return states
}
