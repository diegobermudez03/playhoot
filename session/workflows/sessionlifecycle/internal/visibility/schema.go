// Package visibility implements Playhoot's own capability-based privacy
// boundary for per-viewer game state: before an authored project()
// function ever runs, Filter constructs a viewer-scoped ProjectInput from
// the full authoritative state and a Game's own declared Schema, so the
// untrusted script is never handed data a viewer is not authorized to see
// in the first place. LeakCheck is a separate, defense-in-depth safety net
// against a filtering bug or a misconfigured Schema - never the primary
// guarantee. See this package's own LOGICAL_CONTRACT.md for the full
// model.
package visibility

import (
	"encoding/json"
	"fmt"
)

// Class classifies one declared path in a Game's own state shape.
type Class string

const (
	// ClassPublic data is included in every viewer's own ProjectionInput.
	ClassPublic Class = "public"
	// ClassPrivate data is included only in the owning viewer's own
	// ProjectionInput - see Rule's own doc comment for how ownership is
	// determined.
	ClassPrivate Class = "private"
	// ClassServerOnly data is never included in any viewer's
	// ProjectionInput, regardless of who is asking.
	ClassServerOnly Class = "server_only"
	// ClassDeclassified data is not read directly from state at all -
	// Playhoot itself computes it from a private Source path (via Derive)
	// and exposes only the computed result to every viewer, never the
	// underlying private value. This is the only sanctioned way to expose
	// an authorized derivative of private data (for example, an opponent's
	// card count) without handing the untrusted script the private value
	// itself.
	ClassDeclassified Class = "declassified"
)

// DeriveFunc names one of a small, fixed, Playhoot-computed primitive a
// ClassDeclassified Rule may use. This set is deliberately not extensible
// by an authored script - only Playhoot's own code ever computes a
// declassified value.
type DeriveFunc string

const (
	// DeriveCount is the source's own array/object element count, or
	// string length. 0 for a missing/null source.
	DeriveCount DeriveFunc = "count"
	// DeriveExists is whether the source is present and non-empty (a
	// non-empty string/array/object, or any non-null scalar).
	DeriveExists DeriveFunc = "exists"
	// DeriveRedacted is a fixed placeholder value, standing in for the
	// source's own presence without revealing its content or size.
	DeriveRedacted DeriveFunc = "redacted"
)

const redactedPlaceholder = "HIDDEN"

// Rule declares one path template's visibility. Path is a dot-separated
// template over state's own JSON shape ("players.0.hand" addresses an
// array element the same way "players.p1.hand" addresses an object key);
// "*" matches any single segment (object key or array index).
//
// At most one "*" is supported per Path - the standard "keyed by owning
// participant" shape (for example "players.*.hand") - so that wildcard's
// own matched segment is unambiguously the owning viewer for a
// ClassPrivate rule. This is a deliberate simplification, not a general
// path language: a game needing more than one independent dynamic
// dimension of ownership within a single Path is not supported by this
// first version.
//
// Derive/Source apply only to ClassDeclassified: Derive names which
// primitive Playhoot computes from Source's own value, and the result is
// written at Path for every viewer. Source is a template with the same
// wildcard convention as Path; if both carry a "*", the same bound segment
// is substituted into both (so "players.*.handCount" derived from
// "players.*.hand" pairs each participant's own count with their own
// hand, never another's).
type Rule struct {
	Path   string     `json:"path"`
	Class  Class      `json:"class"`
	Derive DeriveFunc `json:"derive,omitempty"`
	Source string     `json:"source,omitempty"`
}

// Schema is one Game's own declared visibility model - the
// ProjectionVisibility addendum to session/docs/GAME_VERSION_ARTIFACT_MODEL.md's
// artifact shape. Default governs any concrete path Rules does not match;
// it should almost always be ClassServerOnly (the safe default: data is
// exposed only because some Rule explicitly says so, never by omission) -
// an empty Default is treated as ClassServerOnly.
type Schema struct {
	Rules   []Rule `json:"rules"`
	Default Class  `json:"default,omitempty"`
}

// defaultClass is schema.Default, normalized to its safe fallback.
func (s Schema) defaultClass() Class {
	if s.Default == "" {
		return ClassServerOnly
	}
	return s.Default
}

// ParseSchema decodes raw (a session_game_version_artifacts.projection_visibility
// value) into a Schema. A nil/empty raw is a valid, maximally restrictive
// Schema: no Rules, Default ClassServerOnly - every path is excluded from
// every viewer's ProjectionInput. This is a deliberate safe default, not an
// error: a Game that declares no visibility schema at all exposes nothing
// through project(), rather than everything.
func ParseSchema(raw json.RawMessage) (Schema, error) {
	if len(raw) == 0 {
		return Schema{}, nil
	}
	var schema Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return Schema{}, fmt.Errorf("visibility: decoding schema: %w", err)
	}
	return schema, nil
}
