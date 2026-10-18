// Package inputs defines the logical inputs a game's SAMPLEs carry: how a
// manifest declares them, which values are valid, and how a SAMPLE becomes
// the full input snapshot, defaults applied.
package inputs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
)

// Platform bounds on what a game may declare and send.
const (
	MaxInputs       = 32
	maxStringLength = 256
)

// Type is the kind of value an input holds.
type Type string

const (
	Boolean Type = "boolean"
	Number  Type = "number"
	Vector2 Type = "vector2"
	String  Type = "string"
	Enum    Type = "enum"
)

// Declaration is one input as a manifest declares it. Default is the value an
// omitted input takes, as JSON; empty means the type's natural zero.
type Declaration struct {
	Name     string
	Type     Type
	Nullable bool
	Min      *float64
	Max      *float64
	Values   []string
	Default  json.RawMessage
}

// ErrInvalidValue is returned for a value an input cannot hold.
var ErrInvalidValue = errors.New("invalid input value")

var namePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// ValidName reports whether name may name a declared input or command: a
// letter, then up to 63 letters, digits or underscores.
func ValidName(name string) bool {
	return namePattern.MatchString(name)
}

// Validate checks declaration's rules and returns it with its effective
// default filled in.
func Validate(declaration Declaration) (Declaration, error) {
	if !ValidName(declaration.Name) {
		return Declaration{}, errors.New("names start with a letter and hold up to 64 letters, digits or underscores")
	}
	switch declaration.Type {
	case Boolean, Vector2, String:
	case Number:
		if declaration.Min != nil && declaration.Max != nil && *declaration.Min > *declaration.Max {
			return Declaration{}, errors.New("min must not exceed max")
		}
	case Enum:
		if len(declaration.Values) == 0 {
			return Declaration{}, errors.New("an enum declares its values")
		}
		seen := make(map[string]bool, len(declaration.Values))
		for _, value := range declaration.Values {
			if value == "" || seen[value] {
				return Declaration{}, errors.New("enum values must be distinct and not empty")
			}
			seen[value] = true
		}
		if len(declaration.Default) == 0 {
			return Declaration{}, errors.New("an enum has no natural zero, so it declares a default")
		}
	default:
		return Declaration{}, fmt.Errorf("unknown type %q", declaration.Type)
	}
	if declaration.Type != Number && (declaration.Min != nil || declaration.Max != nil) {
		return Declaration{}, errors.New("only number inputs take min and max")
	}
	if declaration.Type != String && declaration.Nullable {
		return Declaration{}, errors.New("only string inputs may be nullable")
	}
	if declaration.Type != Enum && len(declaration.Values) > 0 {
		return Declaration{}, errors.New("only enum inputs take values")
	}

	declared := len(declaration.Default) > 0
	if !declared {
		declaration.Default = naturalZero(declaration)
	}
	canonical, err := canonicalValue(declaration, declaration.Default)
	if err != nil {
		if !declared {
			return Declaration{}, fmt.Errorf("its natural zero %s is not a valid value, so it declares a default", declaration.Default)
		}
		return Declaration{}, fmt.Errorf("default: %w", err)
	}
	declaration.Default = canonical
	return declaration, nil
}

// naturalZero is the value an input takes when nothing else is declared.
func naturalZero(declaration Declaration) json.RawMessage {
	switch declaration.Type {
	case Boolean:
		return json.RawMessage(`false`)
	case Number:
		return json.RawMessage(`0`)
	case Vector2:
		return json.RawMessage(`[0,0]`)
	case String:
		if declaration.Nullable {
			return json.RawMessage(`null`)
		}
		return json.RawMessage(`""`)
	default:
		return nil
	}
}

// canonicalValue checks value against declaration and returns it in one
// canonical encoding, so equal values have equal bytes.
func canonicalValue(declaration Declaration, value json.RawMessage) (json.RawMessage, error) {
	invalid := func(reason string) (json.RawMessage, error) {
		return nil, fmt.Errorf("%w: %s %s", ErrInvalidValue, declaration.Name, reason)
	}
	switch declaration.Type {
	case Boolean:
		var b bool
		if json.Unmarshal(value, &b) != nil {
			return invalid("must be a boolean")
		}
		return json.Marshal(b)
	case Number:
		number, ok := finiteNumber(value)
		if !ok {
			return invalid("must be a finite number")
		}
		if (declaration.Min != nil && number < *declaration.Min) || (declaration.Max != nil && number > *declaration.Max) {
			return invalid("is out of range")
		}
		return json.Marshal(number)
	case Vector2:
		var pair []json.RawMessage
		if json.Unmarshal(value, &pair) != nil || len(pair) != 2 {
			return invalid("must be two numbers")
		}
		x, okX := finiteNumber(pair[0])
		y, okY := finiteNumber(pair[1])
		if !okX || !okY {
			return invalid("must be two finite numbers")
		}
		return json.Marshal([2]float64{x, y})
	case String:
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			if !declaration.Nullable {
				return invalid("must be a string")
			}
			return json.RawMessage(`null`), nil
		}
		var s string
		if json.Unmarshal(value, &s) != nil {
			return invalid("must be a string")
		}
		if len(s) > maxStringLength {
			return invalid(fmt.Sprintf("must hold at most %d bytes", maxStringLength))
		}
		return json.Marshal(s)
	case Enum:
		var s string
		if json.Unmarshal(value, &s) == nil {
			for _, allowed := range declaration.Values {
				if s == allowed {
					return json.Marshal(s)
				}
			}
		}
		return invalid("must be one of its values")
	default:
		return invalid("has an unknown type")
	}
}

func finiteNumber(value json.RawMessage) (float64, bool) {
	var number float64
	if json.Unmarshal(value, &number) != nil || math.IsInf(number, 0) || math.IsNaN(number) {
		return 0, false
	}
	return number, true
}

// Schema is a game's validated input declarations.
type Schema struct {
	declarations map[string]Declaration
	names        []string
}

// NewSchema validates declarations into a Schema.
func NewSchema(declarations []Declaration) (Schema, error) {
	if len(declarations) > MaxInputs {
		return Schema{}, fmt.Errorf("at most %d inputs, got %d", MaxInputs, len(declarations))
	}
	schema := Schema{declarations: make(map[string]Declaration, len(declarations))}
	for _, declaration := range declarations {
		validated, err := Validate(declaration)
		if err != nil {
			return Schema{}, fmt.Errorf("input %q: %w", declaration.Name, err)
		}
		if _, repeated := schema.declarations[validated.Name]; repeated {
			return Schema{}, fmt.Errorf("input %q is declared twice", validated.Name)
		}
		schema.declarations[validated.Name] = validated
		schema.names = append(schema.names, validated.Name)
	}
	sort.Strings(schema.names)
	return schema, nil
}

// Empty reports whether the game declares no input, so it takes no SAMPLE.
func (s Schema) Empty() bool {
	return len(s.names) == 0
}

// Snapshot is a full logical input state in canonical JSON: every declared
// input, in name order, so equal states have equal bytes.
type Snapshot string

// Defaults is the snapshot of a participant who sends nothing.
func (s Schema) Defaults() Snapshot {
	snapshot, _ := s.Normalize([]byte(`{}`))
	return snapshot
}

// Normalize turns a SAMPLE's inputs, a JSON object of declared inputs, into
// the full snapshot. An undeclared input or an invalid value wraps
// ErrInvalidValue.
func (s Schema) Normalize(raw []byte) (Snapshot, error) {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil || given == nil {
		return "", fmt.Errorf("%w: inputs must be a JSON object", ErrInvalidValue)
	}
	for name := range given {
		if _, declared := s.declarations[name]; !declared {
			return "", fmt.Errorf("%w: %s is not a declared input", ErrInvalidValue, name)
		}
	}
	var out bytes.Buffer
	out.WriteByte('{')
	for i, name := range s.names {
		declaration := s.declarations[name]
		value := declaration.Default
		if givenValue, ok := given[name]; ok {
			canonical, err := canonicalValue(declaration, givenValue)
			if err != nil {
				return "", err
			}
			value = canonical
		}
		if i > 0 {
			out.WriteByte(',')
		}
		key, _ := json.Marshal(name)
		out.Write(key)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return Snapshot(out.String()), nil
}

// Declarations returns the validated declarations, defaults filled in, in
// name order.
func (s Schema) Declarations() []Declaration {
	if len(s.names) == 0 {
		return nil
	}
	declarations := make([]Declaration, len(s.names))
	for i, name := range s.names {
		declarations[i] = s.declarations[name]
	}
	return declarations
}
