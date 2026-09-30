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
func matchTemplate(template, concrete []string) (bound string, ok bool) {
	if len(template) != len(concrete) {
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
// containers as needed. Only object (map) intermediate containers are
// created - a synthesized declassified field is always addressed through
// object keys, never array indices, a deliberate v1 restriction mirroring
// Rule's own single-wildcard simplification.
func setByPath(root map[string]interface{}, path []string, value interface{}) {
	cur := root
	for i, seg := range path {
		if i == len(path)-1 {
			cur[seg] = value
			return
		}
		next, ok := cur[seg].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			cur[seg] = next
		}
		cur = next
	}
}
