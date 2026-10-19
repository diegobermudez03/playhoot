// Package jsonpatch computes and applies RFC 6902 JSON Patches between two
// JSON documents, using only add, remove and replace. Unlike a merge patch,
// it represents removed fields, nulls and changed lists exactly.
package jsonpatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Operation kinds.
const (
	OpAdd     = "add"
	OpRemove  = "remove"
	OpReplace = "replace"
)

// Operation is one RFC 6902 operation. Value is meaningless for remove.
type Operation struct {
	Op    string
	Path  string
	Value any
}

// MarshalJSON writes value for add and replace even when it is null.
func (o Operation) MarshalJSON() ([]byte, error) {
	if o.Op == OpRemove {
		return json.Marshal(struct {
			Op   string `json:"op"`
			Path string `json:"path"`
		}{o.Op, o.Path})
	}
	return json.Marshal(struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value"`
	}{o.Op, o.Path, o.Value})
}

// UnmarshalJSON reads an operation, keeping numbers as written.
func (o *Operation) UnmarshalJSON(raw []byte) error {
	var document struct {
		Op    string          `json:"op"`
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	o.Op, o.Path, o.Value = document.Op, document.Path, nil
	if o.Op == OpRemove {
		return nil
	}
	if document.Value == nil {
		return fmt.Errorf("%s at %q has no value", o.Op, o.Path)
	}
	value, err := Parse(document.Value)
	if err != nil {
		return err
	}
	o.Value = value
	return nil
}

var errInvalidPath = errors.New("invalid patch path")

// Parse decodes a JSON document, keeping numbers exactly as written.
func Parse(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}
	if decoder.More() {
		return nil, errors.New("decoding JSON: trailing data")
	}
	return value, nil
}

// Diff returns the operations turning before into after, both as Parse
// returns them; none when they are equal. The operations share values with
// after, which must not change while they are used.
func Diff(before any, after any) []Operation {
	var operations []Operation
	diff("", before, after, &operations)
	return operations
}

func diff(path string, before any, after any, operations *[]Operation) {
	beforeObject, wasObject := before.(map[string]any)
	afterObject, isObject := after.(map[string]any)
	if wasObject && isObject {
		diffObjects(path, beforeObject, afterObject, operations)
		return
	}
	beforeList, wasList := before.([]any)
	afterList, isList := after.([]any)
	if wasList && isList {
		diffLists(path, beforeList, afterList, operations)
		return
	}
	if !reflect.DeepEqual(before, after) {
		*operations = append(*operations, Operation{Op: OpReplace, Path: path, Value: after})
	}
}

// diffObjects visits keys in order, so equal inputs give equal patches.
func diffObjects(path string, before map[string]any, after map[string]any, operations *[]Operation) {
	for _, key := range sortedKeys(before) {
		if _, kept := after[key]; !kept {
			*operations = append(*operations, Operation{Op: OpRemove, Path: path + "/" + escape(key)})
		}
	}
	for _, key := range sortedKeys(after) {
		old, existed := before[key]
		if !existed {
			*operations = append(*operations, Operation{Op: OpAdd, Path: path + "/" + escape(key), Value: after[key]})
			continue
		}
		diff(path+"/"+escape(key), old, after[key], operations)
	}
}

// diffLists patches shared positions in place, then removes the extra tail
// from its end or appends the new one.
func diffLists(path string, before []any, after []any, operations *[]Operation) {
	shared := min(len(before), len(after))
	for i := range shared {
		diff(path+"/"+strconv.Itoa(i), before[i], after[i], operations)
	}
	for i := len(before) - 1; i >= shared; i-- {
		*operations = append(*operations, Operation{Op: OpRemove, Path: path + "/" + strconv.Itoa(i)})
	}
	for i := shared; i < len(after); i++ {
		*operations = append(*operations, Operation{Op: OpAdd, Path: path + "/" + strconv.Itoa(i), Value: after[i]})
	}
}

func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// Apply returns document, as Parse returns it, with operations applied in
// order. document may be changed in place.
func Apply(document any, operations []Operation) (any, error) {
	for _, operation := range operations {
		var err error
		document, err = apply(document, operation)
		if err != nil {
			return nil, fmt.Errorf("applying %s at %q: %w", operation.Op, operation.Path, err)
		}
	}
	return document, nil
}

func apply(document any, operation Operation) (any, error) {
	tokens, err := split(operation.Path)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		switch operation.Op {
		case OpAdd, OpReplace:
			return operation.Value, nil
		default:
			return nil, errors.New("cannot remove the whole document")
		}
	}
	parent, err := walk(document, tokens[:len(tokens)-1])
	if err != nil {
		return nil, err
	}
	last := tokens[len(tokens)-1]
	switch container := parent.value.(type) {
	case map[string]any:
		_, exists := container[last]
		switch {
		case operation.Op == OpAdd:
			container[last] = operation.Value
		case !exists:
			return nil, fmt.Errorf("%w: no field %q", errInvalidPath, last)
		case operation.Op == OpRemove:
			delete(container, last)
		case operation.Op == OpReplace:
			container[last] = operation.Value
		default:
			return nil, fmt.Errorf("unsupported operation %q", operation.Op)
		}
		return document, nil
	case []any:
		list, err := applyToList(container, last, operation)
		if err != nil {
			return nil, err
		}
		return parent.set(document, list), nil
	default:
		return nil, fmt.Errorf("%w: %q is not inside an object or list", errInvalidPath, operation.Path)
	}
}

func applyToList(list []any, token string, operation Operation) ([]any, error) {
	if operation.Op == OpAdd && token == "-" {
		return append(list, operation.Value), nil
	}
	index, err := strconv.Atoi(token)
	limit := len(list)
	if operation.Op != OpAdd {
		limit--
	}
	if err != nil || index < 0 || index > limit {
		return nil, fmt.Errorf("%w: index %q out of range", errInvalidPath, token)
	}
	switch operation.Op {
	case OpAdd:
		return slices.Insert(list, index, operation.Value), nil
	case OpRemove:
		return slices.Delete(list, index, index+1), nil
	case OpReplace:
		list[index] = operation.Value
		return list, nil
	default:
		return nil, fmt.Errorf("unsupported operation %q", operation.Op)
	}
}

// location is a value inside a document and how to put a new one in its
// place, since a changed list may be a new slice.
type location struct {
	value any
	set   func(document any, value any) any
}

func walk(document any, tokens []string) (location, error) {
	current := location{value: document, set: func(_ any, value any) any { return value }}
	for _, token := range tokens {
		switch container := current.value.(type) {
		case map[string]any:
			child, found := container[token]
			if !found {
				return location{}, fmt.Errorf("%w: no field %q", errInvalidPath, token)
			}
			current = location{value: child, set: func(document any, value any) any {
				container[token] = value
				return document
			}}
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(container) {
				return location{}, fmt.Errorf("%w: index %q out of range", errInvalidPath, token)
			}
			current = location{value: container[index], set: func(document any, value any) any {
				container[index] = value
				return document
			}}
		default:
			return location{}, fmt.Errorf("%w: %q is inside a scalar", errInvalidPath, token)
		}
	}
	return current, nil
}

// split turns a JSON Pointer into its unescaped tokens.
func split(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("%w: %q does not start with /", errInvalidPath, path)
	}
	tokens := strings.Split(path[1:], "/")
	for i, token := range tokens {
		tokens[i] = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
	}
	return tokens, nil
}

func escape(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}
