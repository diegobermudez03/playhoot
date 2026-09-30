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
// A non-ClassDeclassified Rule's own Path must never be a prefix of
// another non-ClassDeclassified Rule's own Path (ParseSchema/Filter/
// LeakCheck all reject such a Schema outright): a matched Path decides its
// whole subtree atomically and is never re-descended into, so a nested
// Rule would be unreachable at best, and at worst - a broader, less
// restrictive Rule sitting above a narrower, more restrictive one - would
// silently expose the narrower Rule's own intended restriction to every
// viewer. Declare Rules at the exact leaf/subtree granularity intended,
// never nested inside another Rule's own Path.
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
//
// An unrecognized Class on a Rule or on Default is rejected outright,
// rather than accepted and left to degrade at Filter/LeakCheck time: a
// Rule whose Class does not match one of the four recognized constants
// (for example, an author's typo) must never be treated as though no Rule
// existed for its Path at all, since that would silently fall through to
// that path's own container/Default-class handling and could expose data
// the author intended to restrict.
func ParseSchema(raw json.RawMessage) (Schema, error) {
	if len(raw) == 0 {
		return Schema{}, nil
	}
	var schema Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return Schema{}, fmt.Errorf("visibility: decoding schema: %w", err)
	}
	if err := schema.validate(); err != nil {
		return Schema{}, err
	}
	return schema, nil
}

// validate rejects a Schema that is unsafe by construction, rather than
// leaving it to degrade silently at Filter/LeakCheck time.
func (s Schema) validate() error {
	if s.Default != "" && !isKnownClass(s.Default) {
		return fmt.Errorf("visibility: schema default %q is not a recognized class", s.Default)
	}

	// nonDeclassified holds each structural Rule's own split Path template
	// alongside its index, for the nesting check below. ClassDeclassified
	// Rules are exempt: matchRule never considers them during Filter's own
	// structural walk (they are synthesized separately, by path/value, not
	// matched against the walk at all), so they cannot participate in the
	// same shadowing hazard a structural Rule's Path can.
	type indexedTemplate struct {
		index    int
		path     string
		template []string
	}
	var nonDeclassified []indexedTemplate

	for i, rule := range s.Rules {
		if !isKnownClass(rule.Class) {
			return fmt.Errorf("visibility: rule %d (path %q) has unrecognized class %q", i, rule.Path, rule.Class)
		}
		// At most one "*" per Path/Source (Rule's own doc comment): a
		// second wildcard has no defined owner-binding meaning at all -
		// matchTemplate can only ever report one bound segment, and
		// without this check it would silently bind to whichever
		// wildcard happens to be evaluated last, an entirely different
		// segment than the template's own intended owner. Rejected
		// outright, the same as every other unsafe construction here.
		if n := wildcardCount(splitPath(rule.Path)); n > 1 {
			return fmt.Errorf("visibility: rule %d (path %q) has %d wildcard segments, but at most one is supported", i, rule.Path, n)
		}
		if rule.Class == ClassDeclassified {
			if n := wildcardCount(splitPath(rule.Source)); n > 1 {
				return fmt.Errorf("visibility: rule %d (source %q) has %d wildcard segments, but at most one is supported", i, rule.Source, n)
			}
			continue
		}
		nonDeclassified = append(nonDeclassified, indexedTemplate{index: i, path: rule.Path, template: splitPath(rule.Path)})
	}

	// A structural Rule's Path must never be nested inside, or exactly
	// duplicate, another structural Rule's own Path: filterValue decides a
	// matched path's whole subtree atomically and never re-descends into
	// it once a shallower Rule already matched (and, for an exact
	// duplicate, matchRule always resolves to whichever Rule appears first
	// in declaration order), so either construction would either be
	// silently unreachable/order-dependent (harmless but pointless), or,
	// worse, one Rule's own intended restriction (server_only, or private
	// to a different owner) would be silently overridden by the other
	// Rule's own, less restrictive class - exposing data across every
	// viewer regardless of what the shadowed Rule itself declared.
	// Rejected outright, the same way an unrecognized Class is, rather
	// than left to degrade silently.
	for a := 0; a < len(nonDeclassified); a++ {
		for b := a + 1; b < len(nonDeclassified); b++ {
			x, y := nonDeclassified[a], nonDeclassified[b]
			if templatesEqual(x.template, y.template) {
				return fmt.Errorf("visibility: rule %d (path %q) and rule %d (path %q) declare the exact same Path - a Schema must not declare the same Path twice", x.index, x.path, y.index, y.path)
			}
			if pathsNest(x.template, y.template) {
				return fmt.Errorf("visibility: rule %d (path %q) and rule %d (path %q) are nested - one Rule's Path must never be a prefix of another Rule's own Path", x.index, x.path, y.index, y.path)
			}
		}
	}

	// A ClassDeclassified Rule's own Source must resolve to data explicitly
	// declared ClassPrivate by some other Rule - never to server_only data,
	// and never to a path no Rule declares at all (which falls back to
	// Default and, for any Default other than ClassPublic, is exactly as
	// restricted as server_only; Default itself carries no per-viewer
	// ownership concept, so it can never itself justify a "private"
	// classification either). applyDeclassifiedRule reads Source directly
	// against the original, unfiltered state, entirely bypassing whatever
	// class governs that same path during the structural walk - so nothing
	// else in this package would otherwise stop a Source pointing at
	// data the Schema itself says must never reach any viewer.
	for i, rule := range s.Rules {
		if rule.Class != ClassDeclassified {
			continue
		}
		sourceTemplate := splitPath(rule.Source)
		if !s.sourceIsExplicitlyPrivate(sourceTemplate) {
			return fmt.Errorf("visibility: rule %d (path %q) derives from source %q, which is not declared private by any Rule - a declassified derivative may only be computed from data explicitly declared private", i, rule.Path, rule.Source)
		}
	}

	return nil
}

// sourceIsExplicitlyPrivate reports whether sourceTemplate is exactly, or
// nested inside, some Rule's own ClassPrivate Path. A ClassPrivate Rule's
// own Path already decides its entire subtree atomically as owned by its
// own bound viewer (the nesting rejection above guarantees no other Rule
// can intercept a path nested inside it), so a Source reaching into that
// same subtree is exactly as private as the field the Rule itself
// governs - not only a Source that names the identical Path.
//
// This uses privateRuleCoversEverySourceInstantiation, not
// isPrefixOrEqual: the two ask genuinely different questions and
// conflating them was itself a real bug, found during independent review.
// isPrefixOrEqual's symmetric wildcard tolerance is exactly right for
// pathsNest (any structural possibility of nesting across any state is a
// hazard), but wrong here - a wildcard on sourceTemplate's own side is
// walked by enumerateInstantiations against every matching key/index in
// the real state at runtime, entirely independent of which single
// instantiation a concrete Rule Path happens to name; forgiving that
// wildcard against a concrete Rule segment (as isPrefixOrEqual would)
// lets a declassified derivative range over participants no Rule ever
// declared private at all.
func (s Schema) sourceIsExplicitlyPrivate(sourceTemplate []string) bool {
	for _, rule := range s.Rules {
		if rule.Class != ClassPrivate {
			continue
		}
		if privateRuleCoversEverySourceInstantiation(splitPath(rule.Path), sourceTemplate) {
			return true
		}
	}
	return false
}

// privateRuleCoversEverySourceInstantiation reports whether privateTemplate
// (a ClassPrivate Rule's own Path) provably covers every concrete
// instantiation sourceTemplate could ever enumerate at runtime. Unlike
// isPrefixOrEqual, this is deliberately asymmetric: privateTemplate may be
// exactly as, or more, general than sourceTemplate at every position (a
// wildcard on privateTemplate's own side covers anything, including a
// wildcard on sourceTemplate's side), but a wildcard on sourceTemplate's
// side is never covered by a concrete/literal segment on privateTemplate's
// side - that concrete segment only ever governs one instantiation, while
// sourceTemplate's own wildcard ranges over every matching key/index
// actually present in state.
func privateRuleCoversEverySourceInstantiation(privateTemplate, sourceTemplate []string) bool {
	if len(privateTemplate) > len(sourceTemplate) {
		return false
	}
	for i, p := range privateTemplate {
		if p == wildcard {
			continue
		}
		if sourceTemplate[i] != p {
			return false
		}
	}
	return true
}

// templatesEqual reports whether a and b are the exact same path template,
// segment for segment (a literal segment must match the same literal, and
// "*" must align with "*" - no wildcard tolerance, unlike pathsNest).
func templatesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// pathsNest reports whether one of a/b is a proper prefix of the other -
// conservative on purpose, since a Schema is declared once and applied
// across arbitrarily many states/viewers: any structural possibility of
// nesting is rejected, not only a nesting that happens to occur for some
// specific state.
func pathsNest(a, b []string) bool {
	shorter, longer := a, b
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) == len(longer) {
		return false
	}
	return isPrefixOrEqual(shorter, longer)
}

// isPrefixOrEqual reports whether prefix is exactly full, or a prefix of
// it, treating "*" as compatible with any segment (including another "*")
// at the same position - conservative on purpose, for the same reason
// pathsNest is: a Schema is declared once and applied across arbitrarily
// many states/viewers, so any structural possibility of containment is
// what matters, not only a containment that happens to hold for some
// specific state.
func isPrefixOrEqual(prefix, full []string) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i, seg := range prefix {
		other := full[i]
		if seg == wildcard || other == wildcard {
			continue
		}
		if seg != other {
			return false
		}
	}
	return true
}

func isKnownClass(c Class) bool {
	switch c {
	case ClassPublic, ClassPrivate, ClassServerOnly, ClassDeclassified:
		return true
	default:
		return false
	}
}
