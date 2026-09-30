package visibility

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// minLeakCheckBytes is the smallest canonical-JSON encoding LeakCheck will
// treat as a meaningful signal. A short scalar (a small number, a boolean,
// a short common string) coinciding between two unrelated values is not
// evidence of a leak and would make this check too noisy to be useful; a
// longer or structured value appearing verbatim is a much stronger signal.
const minLeakCheckBytes = 8

// LeakCheck reports every excluded-from-viewer subtree of state (per
// schema - private data owned by someone else, or server_only data) whose
// own canonical JSON encoding appears verbatim inside clientState
// (project()'s own output for viewer). It is a defense-in-depth safety net
// against a Filter-construction bug or a misconfigured Schema - privacy is
// primarily guaranteed by Filter never handing project() that data at all;
// this check is secondary, catching a bug in that construction rather than
// substituting for it. A non-empty result does not prove a leak (see
// minLeakCheckBytes and this package's own LOGICAL_CONTRACT.md for its
// documented limitations); it is a signal a caller should alert on, not
// silently ignore.
func LeakCheck(state json.RawMessage, viewer string, clientState json.RawMessage, schema Schema) ([]string, error) {
	var root interface{}
	if len(state) > 0 {
		if err := json.Unmarshal(state, &root); err != nil {
			return nil, fmt.Errorf("visibility: decoding state: %w", err)
		}
	}
	var clientValue interface{}
	if len(clientState) > 0 {
		if err := json.Unmarshal(clientState, &clientValue); err != nil {
			return nil, fmt.Errorf("visibility: decoding client state: %w", err)
		}
	}
	clientCanonical, err := json.Marshal(clientValue)
	if err != nil {
		return nil, fmt.Errorf("visibility: encoding client state: %w", err)
	}

	var excluded []interface{}
	collectExcluded(root, nil, schema, viewer, &excluded)

	var findings []string
	for _, val := range excluded {
		encoded, err := json.Marshal(val)
		if err != nil {
			continue
		}
		if len(encoded) < minLeakCheckBytes {
			continue
		}
		if bytes.Contains(clientCanonical, encoded) {
			findings = append(findings, string(encoded))
		}
	}
	return findings, nil
}

// collectExcluded walks value the same way filterValue does, at the exact
// same decision granularity, but collects whichever whole subtrees Filter
// would have excluded instead of building the included tree - so a
// private-not-owned subtree is collected once, as a whole value, not one
// finding per leaf.
func collectExcluded(value interface{}, path []string, schema Schema, viewer string, out *[]interface{}) {
	if rule, bound, ok := matchRule(schema.Rules, path); ok {
		switch rule.Class {
		case ClassPublic:
			return
		case ClassPrivate:
			if bound != viewer {
				*out = append(*out, value)
			}
			return
		case ClassServerOnly:
			*out = append(*out, value)
			return
		}
	}

	switch v := value.(type) {
	case map[string]interface{}:
		for k, child := range v {
			collectExcluded(child, append(path, k), schema, viewer, out)
		}
	case []interface{}:
		for i, child := range v {
			collectExcluded(child, append(path, strconv.Itoa(i)), schema, viewer, out)
		}
	default:
		if schema.defaultClass() != ClassPublic {
			*out = append(*out, value)
		}
	}
}
