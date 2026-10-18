// Package manifest reads a game version's runtime manifest: the structural
// facts the live Runtime needs about a game, stored as versioned JSON.
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
)

// SchemaVersion is the only manifest schema this domain reads.
const SchemaVersion = 1

// Manifest is a decoded, valid runtime manifest.
type Manifest struct {
	MinPlayers int
	MaxPlayers int
}

// ErrInvalid is returned for a manifest that cannot be decoded or breaks a
// rule; errors wrap it with the reason.
var ErrInvalid = errors.New("invalid runtime manifest")

type document struct {
	SchemaVersion int `json:"schema_version"`
	Players       *struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"players"`
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
	return Manifest{MinPlayers: doc.Players.Min, MaxPlayers: doc.Players.Max}, nil
}
