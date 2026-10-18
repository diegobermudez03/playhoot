// Package viewpatch computes what a participant's view changed since the last
// one their connection received. A view's shape is fixed: fields never
// disappear, so a patch only carries fields with new values.
package viewpatch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// Diff returns the patch turning previous into next: an object with only the
// changed fields, nested objects patched the same way and anything else,
// lists included, sent whole. full is true when next must be sent whole
// instead: there is no previous view, either view is not an object, or a
// field disappeared.
func Diff(previous []byte, next []byte) (patch []byte, full bool, err error) {
	if len(previous) == 0 {
		return next, true, nil
	}
	before, err := decode(previous)
	if err != nil {
		return nil, false, fmt.Errorf("decoding previous view: %w", err)
	}
	after, err := decode(next)
	if err != nil {
		return nil, false, fmt.Errorf("decoding view: %w", err)
	}
	beforeObject, ok := before.(map[string]any)
	afterObject, alsoOK := after.(map[string]any)
	if !ok || !alsoOK {
		return next, true, nil
	}
	changes, removed := diffObjects(beforeObject, afterObject)
	if removed {
		return next, true, nil
	}
	encoded, err := json.Marshal(changes)
	if err != nil {
		return nil, false, fmt.Errorf("encoding patch: %w", err)
	}
	return encoded, false, nil
}

// diffObjects returns the fields of after that differ from before; removed
// is true when a field of before is missing from after.
func diffObjects(before map[string]any, after map[string]any) (changes map[string]any, removed bool) {
	for key := range before {
		if _, kept := after[key]; !kept {
			return nil, true
		}
	}
	changes = make(map[string]any)
	for key, value := range after {
		old := before[key]
		oldObject, wasObject := old.(map[string]any)
		newObject, isObject := value.(map[string]any)
		if wasObject && isObject {
			nested, nestedRemoved := diffObjects(oldObject, newObject)
			if nestedRemoved {
				return nil, true
			}
			if len(nested) > 0 {
				changes[key] = nested
			}
			continue
		}
		if !reflect.DeepEqual(old, value) {
			changes[key] = value
		}
	}
	return changes, false
}

func decode(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	// Numbers stay as written, so equal views compare equal exactly.
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}
