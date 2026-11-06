package scenariodoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Segment is one step of a path: an object key, or an array index when
// IsIndex.
type Segment struct {
	Key     string
	Index   int
	IsIndex bool
}

// maxPathLength bounds a path.
const maxPathLength = 512

// ParsePath reads a path into a JSON value: keys separated by dots and array
// indexes in brackets, as in players.P1.hand[0].rank. Empty is the whole
// value.
func ParsePath(path string) ([]Segment, error) {
	if len(path) > maxPathLength {
		return nil, fmt.Errorf("a path holds at most %d characters", maxPathLength)
	}
	var segments []Segment
	if path == "" {
		return segments, nil
	}
	for _, part := range strings.Split(path, ".") {
		key, rest, _ := strings.Cut(part, "[")
		if key == "" && !strings.HasPrefix(part, "[") {
			return nil, errors.New(`a path is keys separated by dots, with [n] for array items, as in "players.P1.score"`)
		}
		if key != "" {
			segments = append(segments, Segment{Key: key})
		}
		if !strings.HasPrefix(part, key+"[") {
			continue
		}
		rest = "[" + rest
		for rest != "" {
			if !strings.HasPrefix(rest, "[") {
				return nil, fmt.Errorf("unexpected %q in the path", rest)
			}
			end := strings.Index(rest, "]")
			if end < 0 {
				return nil, errors.New("a [ in the path is not closed")
			}
			index, err := strconv.Atoi(rest[1:end])
			if err != nil || index < 0 {
				return nil, fmt.Errorf("%q is not an array index", rest[1:end])
			}
			segments = append(segments, Segment{Index: index, IsIndex: true})
			rest = rest[end+1:]
		}
	}
	return segments, nil
}

// Resolve is the value at segments inside document, or false when it has
// none there.
func Resolve(document json.RawMessage, segments []Segment) (json.RawMessage, bool) {
	value := document
	for _, segment := range segments {
		trimmed := bytes.TrimSpace(value)
		if segment.IsIndex {
			var items []json.RawMessage
			if len(trimmed) == 0 || trimmed[0] != '[' || json.Unmarshal(trimmed, &items) != nil || segment.Index >= len(items) {
				return nil, false
			}
			value = items[segment.Index]
			continue
		}
		var fields map[string]json.RawMessage
		if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &fields) != nil {
			return nil, false
		}
		next, found := fields[segment.Key]
		if !found {
			return nil, false
		}
		value = next
	}
	if len(bytes.TrimSpace(value)) == 0 {
		return nil, false
	}
	return value, true
}
