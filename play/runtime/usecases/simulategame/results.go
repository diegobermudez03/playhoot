package simulategame

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"

	"github.com/diegobermudez03/playhoot/play/internal/business/scenariodoc"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// Limits of the kept results, which Play's Session domain stores.
const (
	// ResultTTL is how long a result can be read again.
	ResultTTL = 30 * time.Minute
	// MaxStateReadBytes bounds a value a state read answers; a larger one
	// answers its size and keys so a narrower path can be asked for.
	MaxStateReadBytes = 64 * 1024
)

// readState is the value at path of one observation of result: the one
// taken for atMs (nil: the final one), of the global state or a
// participant's view.
func readState(result runtime.SimulationResult, atMs *int64, of string, path string) runtime.SimulationStateRead {
	read := runtime.SimulationStateRead{Of: of, Path: path}
	segments, err := scenariodoc.ParsePath(path)
	if err != nil {
		read.Outcome, read.Problem = runtime.StateReadInvalid, err.Error()
		return read
	}
	if of != scenariodoc.OfGlobal && !contains(result.Players, of) {
		read.Outcome, read.Problem = runtime.StateReadInvalid, `of is "global" or a participant of the simulation`
		return read
	}
	observation, final, found := observationAt(&result, atMs)
	if !found {
		read.Outcome = runtime.StateReadNotObserved
		for _, taken := range result.Observations {
			read.Observed = append(read.Observed, taken.RequestedAtMs)
		}
		return read
	}
	read.RequestedAtMs, read.AtMs, read.Final = observation.RequestedAtMs, observation.AtMs, final
	document := observation.State
	if of != scenariodoc.OfGlobal {
		document = observation.Views[of]
	}
	value, found := scenariodoc.Resolve(document, segments)
	if !found {
		read.Outcome = runtime.StateReadPathNotFound
		return read
	}
	read.Outcome = runtime.StateReadFound
	if len(value) <= MaxStateReadBytes {
		read.Value = value
		return read
	}
	read.TooLarge, read.Size = true, len(value)
	trimmed := bytes.TrimSpace(value)
	switch trimmed[0] {
	case '{':
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(trimmed, &fields)
		for key := range fields {
			read.Keys = append(read.Keys, key)
		}
		sort.Strings(read.Keys)
	case '[':
		var items []json.RawMessage
		_ = json.Unmarshal(trimmed, &items)
		length := len(items)
		read.Length = &length
	}
	return read
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
