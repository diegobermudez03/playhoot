// Package manifest reads a game version's runtime manifest: the structural
// facts the live Runtime needs about a game, stored as versioned JSON.
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/internal/business/commands"
	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
)

// SchemaVersion is the only manifest schema this domain reads.
const SchemaVersion = 1

// Manifest is a decoded, valid runtime manifest. Inputs and command payloads
// hold their effective defaults and are sorted by name.
type Manifest struct {
	MinPlayers int
	MaxPlayers int
	Inputs     []inputs.Declaration
	Commands   []commands.Declaration

	// Update means the game implements update(), called periodically.
	Update bool
}

// entrypointUpdate is the optional update() entrypoint, as entrypoints
// declares it.
const entrypointUpdate = "update"

// ErrInvalid is returned for a manifest that cannot be decoded or breaks a
// rule; errors wrap it with the reason.
var ErrInvalid = errors.New("invalid runtime manifest")

type document struct {
	SchemaVersion int `json:"schema_version"`
	Players       *struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"players"`
	Inputs      map[string]inputDocument   `json:"inputs"`
	Commands    map[string]commandDocument `json:"commands"`
	Entrypoints []string                   `json:"entrypoints"`
}

type commandDocument struct {
	Payload map[string]inputDocument `json:"payload"`
}

type inputDocument struct {
	Type     inputs.Type     `json:"type"`
	Nullable bool            `json:"nullable"`
	Min      *float64        `json:"min"`
	Max      *float64        `json:"max"`
	Values   []string        `json:"values"`
	Default  json.RawMessage `json:"default"`
}

// Parse decodes raw and checks every rule of SchemaVersion. Fields it does not
// know are ignored.
func Parse(raw []byte) (Manifest, error) {
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if doc.SchemaVersion != SchemaVersion {
		return Manifest{}, fmt.Errorf("%w: schema_version %d is not %d", ErrInvalid, doc.SchemaVersion, SchemaVersion)
	}
	if doc.Players == nil {
		return Manifest{}, fmt.Errorf("%w: players is required", ErrInvalid)
	}
	if doc.Players.Min < 1 || doc.Players.Max < doc.Players.Min {
		return Manifest{}, fmt.Errorf("%w: players must satisfy 1 <= min <= max, got min %d max %d", ErrInvalid, doc.Players.Min, doc.Players.Max)
	}

	schema, err := inputs.NewSchema(inputDeclarations(doc.Inputs))
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	declaredCommands := make([]commands.Declaration, 0, len(doc.Commands))
	for name, declared := range doc.Commands {
		declaredCommands = append(declaredCommands, commands.Declaration{Name: name, Payload: inputDeclarations(declared.Payload)})
	}
	catalog, err := commands.NewCatalog(declaredCommands)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	update := false
	for _, entrypoint := range doc.Entrypoints {
		if entrypoint != entrypointUpdate {
			return Manifest{}, fmt.Errorf("%w: unknown optional entrypoint %q", ErrInvalid, entrypoint)
		}
		if update {
			return Manifest{}, fmt.Errorf("%w: entrypoint %q is declared twice", ErrInvalid, entrypoint)
		}
		update = true
	}
	return Manifest{
		MinPlayers: doc.Players.Min,
		MaxPlayers: doc.Players.Max,
		Inputs:     schema.Declarations(),
		Commands:   catalog.Declarations(),
		Update:     update,
	}, nil
}

func inputDeclarations(documents map[string]inputDocument) []inputs.Declaration {
	declarations := make([]inputs.Declaration, 0, len(documents))
	for name, declared := range documents {
		declarations = append(declarations, inputs.Declaration{
			Name: name, Type: declared.Type, Nullable: declared.Nullable,
			Min: declared.Min, Max: declared.Max, Values: declared.Values, Default: declared.Default,
		})
	}
	return declarations
}
