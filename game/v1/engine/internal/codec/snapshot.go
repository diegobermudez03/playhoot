package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

type snapshotWire struct {
	GlobalState json.RawMessage `json:"global_state"`
	Root        json.RawMessage `json:"root"`
	Random      randomStateWire `json:"random"`
	Sequence    uint64          `json:"sequence"`
}

type randomStateWire struct {
	State uint64 `json:"state"`
}

// EncodeSnapshot encodes snapshot as JSON.
func EncodeSnapshot(path string, snapshot engine.Snapshot) (json.RawMessage, error) {
	global, err := EncodeValue(pathField(path, "global_state"), snapshot.GlobalState)
	if err != nil {
		return nil, err
	}
	root, err := EncodeWorkflowInstance(pathField(path, "root"), snapshot.Root)
	if err != nil {
		return nil, err
	}
	return json.Marshal(snapshotWire{
		GlobalState: global,
		Root:        root,
		Random:      randomStateWire{State: snapshot.Random.State},
		Sequence:    snapshot.Sequence,
	})
}

// DecodeSnapshot decodes data as an engine.Snapshot. It requires data
// to hold exactly one JSON object; every nested value is decoded
// strictly, rejecting unknown fields and unrecognized discriminators,
// so a structural problem anywhere is reported as a path-aware
// *DecodeError rooted at path.
func DecodeSnapshot(path string, data []byte) (engine.Snapshot, error) {
	raw, err := decodeTopLevelValue(path, data)
	if err != nil {
		return engine.Snapshot{}, err
	}
	var w snapshotWire
	if err := strictDecodeInto(path, raw, &w); err != nil {
		return engine.Snapshot{}, err
	}

	globalValue, err := DecodeValue(pathField(path, "global_state"), w.GlobalState)
	if err != nil {
		return engine.Snapshot{}, err
	}
	global, ok := globalValue.(engine.RecordValue)
	if globalValue != nil && !ok {
		return engine.Snapshot{}, newDecodeError(pathField(path, "global_state"), "expected a record value", nil)
	}

	root, err := DecodeWorkflowInstance(pathField(path, "root"), w.Root)
	if err != nil {
		return engine.Snapshot{}, err
	}

	return engine.Snapshot{
		GlobalState: global,
		Root:        root,
		Random:      engine.RandomState{State: w.Random.State},
		Sequence:    w.Sequence,
	}, nil
}
