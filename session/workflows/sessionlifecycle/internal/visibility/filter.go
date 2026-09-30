package visibility

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Filter constructs viewer's own ProjectionInput from the full
// authoritative state, applying schema's declared classes: public data is
// always included, private data only when its matched owner equals viewer,
// server_only data is never included for any viewer, and declassified data
// is computed by this function itself (never authored script code) and
// included for every viewer regardless of who owns its Source. Any
// concrete path schema's Rules do not match falls back to
// schema.Default (ClassServerOnly if unset) - see Schema's own doc comment.
//
// This is the primary privacy guarantee: the returned value is the only
// state a subsequent project() call ever receives. It is never handed the
// full state and trusted to filter itself.
func Filter(state json.RawMessage, viewer string, schema Schema) (json.RawMessage, error) {
	var root interface{}
	if len(state) > 0 {
		if err := json.Unmarshal(state, &root); err != nil {
			return nil, fmt.Errorf("visibility: decoding state: %w", err)
		}
	}

	filtered, included := filterValue(root, nil, schema, viewer)
	out, ok := filtered.(map[string]interface{})
	if !included || !ok {
		// A non-object root, or a root with nothing to include, has no
		// named paths a per-path Schema can address - an empty object is
		// the correct, safe projection input either way.
		out = map[string]interface{}{}
	}

	for _, rule := range schema.Rules {
		if rule.Class != ClassDeclassified {
			continue
		}
		applyDeclassifiedRule(root, out, rule)
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("visibility: encoding projection input: %w", err)
	}
	return encoded, nil
}

// matchRule returns the first Rule whose Path exactly matches path, and
// the segment its wildcard (if any) bound. ClassDeclassified rules never
// match here - they are synthesized separately (applyDeclassifiedRule),
// never decided during this structural walk, since a declassified path
// commonly does not exist in state's own shape at all.
func matchRule(rules []Rule, path []string) (Rule, string, bool) {
	for _, rule := range rules {
		if rule.Class == ClassDeclassified {
			continue
		}
		if bound, ok := matchTemplate(splitPath(rule.Path), path); ok {
			return rule, bound, true
		}
	}
	return Rule{}, "", false
}

// filterValue decides value (found at path within the original state tree)
// and returns the value to keep plus whether to include it at all. A
// container (map/slice) with no matched rule at its own exact path is
// never itself decided - it is reconstructed from whichever of its own
// children are individually included, exactly the granularity a Rule's own
// Path is declared at.
func filterValue(value interface{}, path []string, schema Schema, viewer string) (interface{}, bool) {
	if rule, bound, ok := matchRule(schema.Rules, path); ok {
		switch rule.Class {
		case ClassPublic:
			return value, true
		case ClassPrivate:
			return value, bound == viewer
		case ClassServerOnly:
			return nil, false
		}
	}

	switch v := value.(type) {
	case map[string]interface{}:
		out := map[string]interface{}{}
		any := false
		for k, child := range v {
			if cv, include := filterValue(child, append(path, k), schema, viewer); include {
				out[k] = cv
				any = true
			}
		}
		if !any {
			return nil, false
		}
		return out, true
	case []interface{}:
		var out []interface{}
		for i, child := range v {
			if cv, include := filterValue(child, append(path, strconv.Itoa(i)), schema, viewer); include {
				out = append(out, cv)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	default:
		if schema.defaultClass() == ClassPublic {
			return value, true
		}
		return nil, false
	}
}

// applyDeclassifiedRule computes rule's own derived value for every
// concrete instantiation of rule.Source actually present in root, and
// writes each result into out at the corresponding instantiation of
// rule.Path. Instantiations are enumerated against Source, never Path,
// because Path commonly names a synthesized field (for example
// "players.*.handCount") that does not itself exist anywhere in state -
// only Source (for example "players.*.hand") corresponds to real data.
// When both carry a "*", the same bound segment from a Source
// instantiation is substituted into Path to produce its own concrete
// destination.
func applyDeclassifiedRule(root interface{}, out map[string]interface{}, rule Rule) {
	pathTemplate := splitPath(rule.Path)
	sourceTemplate := splitPath(rule.Source)
	pathWildcard := wildcardIndex(pathTemplate)
	sourceWildcard := wildcardIndex(sourceTemplate)

	for _, concreteSource := range enumerateInstantiations(root, sourceTemplate) {
		concretePath := pathTemplate
		if pathWildcard >= 0 && sourceWildcard >= 0 {
			bound := concreteSource[sourceWildcard]
			concretePath = append([]string{}, pathTemplate...)
			concretePath[pathWildcard] = bound
		}
		sourceValue, _ := getByPath(root, concreteSource)
		setByPath(out, concretePath, derive(rule.Derive, sourceValue))
	}
}

// enumerateInstantiations returns every concrete path matching template
// that actually exists in root, substituting template's own single "*"
// (if any) with each key/index actually present at that position.
func enumerateInstantiations(root interface{}, template []string) [][]string {
	var results [][]string
	var walk func(node interface{}, idx int, acc []string)
	walk = func(node interface{}, idx int, acc []string) {
		if idx == len(template) {
			results = append(results, append([]string{}, acc...))
			return
		}
		seg := template[idx]
		if seg == wildcard {
			switch v := node.(type) {
			case map[string]interface{}:
				for k, child := range v {
					walk(child, idx+1, append(acc, k))
				}
			case []interface{}:
				for i, child := range v {
					walk(child, idx+1, append(acc, strconv.Itoa(i)))
				}
			}
			return
		}
		switch v := node.(type) {
		case map[string]interface{}:
			if child, ok := v[seg]; ok {
				walk(child, idx+1, append(acc, seg))
			}
		case []interface{}:
			i, err := strconv.Atoi(seg)
			if err == nil && i >= 0 && i < len(v) {
				walk(v[i], idx+1, append(acc, seg))
			}
		}
	}
	walk(root, 0, nil)
	return results
}

func derive(fn DeriveFunc, value interface{}) interface{} {
	switch fn {
	case DeriveCount:
		return countOf(value)
	case DeriveExists:
		return existsOf(value)
	case DeriveRedacted:
		return redactedPlaceholder
	default:
		return nil
	}
}

func countOf(value interface{}) float64 {
	switch v := value.(type) {
	case []interface{}:
		return float64(len(v))
	case map[string]interface{}:
		return float64(len(v))
	case string:
		return float64(len(v))
	default:
		return 0
	}
}

func existsOf(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	default:
		return true
	}
}
