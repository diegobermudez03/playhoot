package visibility

import (
	"strconv"
	"strings"
)

const wildcard = "*"

// splitPath parses a dot-separated path template/concrete path into its
// segments.
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

// matchTemplate reports whether template (at most one "*") matches concrete
// exactly (same length, every non-wildcard segment equal), and if so
// returns the wildcard's own bound segment ("" if template has none).
//
// A template with more than one "*" never matches - fails closed - rather
// than silently binding to only its last wildcard's own segment (which
// would misattribute ownership to whatever happened to occupy that later
// position, not the template's real intended owner). Schema.validate
// already rejects such a template outright before it can reach here; this
// is defense-in-depth for a caller that builds a Rule without going
// through it, the same layering already applied to every other unsafe
// construction this package rejects.
func matchTemplate(template, concrete []string) (bound string, ok bool) {
	if len(template) != len(concrete) {
		return "", false
	}
	if wildcardCount(template) > 1 {
		return "", false
	}
	for i, seg := range template {
		if seg == wildcard {
			bound = concrete[i]
			continue
		}
		if seg != concrete[i] {
			return "", false
		}
	}
	return bound, true
}

// wildcardIndex returns template's own single "*" segment index, or -1 if
// it has none.
func wildcardIndex(template []string) int {
	for i, seg := range template {
		if seg == wildcard {
			return i
		}
	}
	return -1
}

// wildcardCount returns how many "*" segments template contains.
func wildcardCount(template []string) int {
	count := 0
	for _, seg := range template {
		if seg == wildcard {
			count++
		}
	}
	return count
}

// getByPath resolves path against root (an already json.Unmarshal'ed
// generic tree: map[string]interface{}/[]interface{}/scalar). Returns
// ok=false if any segment does not resolve.
func getByPath(root interface{}, path []string) (interface{}, bool) {
	cur := root
	for _, seg := range path {
		switch v := cur.(type) {
		case map[string]interface{}:
			child, ok := v[seg]
			if !ok {
				return nil, false
			}
			cur = child
		case []interface{}:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// setByPath writes value at path within root, creating intermediate object
// containers as needed, and reports whether the write succeeded. Only
// object (map) intermediate containers are created or descended into - a
// synthesized declassified field is always addressed through object keys,
// never array indices, a deliberate v1 restriction mirroring Rule's own
// single-wildcard simplification.
//
// If an intermediate segment already holds a non-map value (for example,
// a JSON array Filter's own structural pass already included there), the
// write is skipped entirely - reports false - rather than destroying that
// existing value by silently replacing it with a fresh empty map. A
// missing declassified field is already a documented, safe outcome
// elsewhere in this package (applyDeclassifiedRule's own doc comment); it
// is far preferable to quietly discarding data the structural pass had
// already legitimately included.
func setByPath(root map[string]interface{}, path []string, value interface{}) bool {
	cur := root
	for i, seg := range path {
		if i == len(path)-1 {
			cur[seg] = value
			return true
		}
		existing, present := cur[seg]
		if !present {
			next := map[string]interface{}{}
			cur[seg] = next
			cur = next
			continue
		}
		next, ok := existing.(map[string]interface{})
		if !ok {
			return false
		}
		cur = next
	}
	return true
}
