// Package commands defines the COMMANDs a game declares: their names, the
// payload each carries, and how a request's payload becomes the full payload
// the game receives. Payload fields follow the rules of SAMPLE inputs.
package commands

import (
	"errors"
	"fmt"
	"sort"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
)

// MaxCommands bounds how many commands a game may declare.
const MaxCommands = 64

// Declaration is one command as a manifest declares it: its name and its
// payload's fields, each declared like a SAMPLE input.
type Declaration struct {
	Name    string
	Payload []inputs.Declaration
}

// ErrUndeclared is returned for a command name the game does not declare.
var ErrUndeclared = errors.New("undeclared command")

// Catalog is a game's validated command declarations.
type Catalog struct {
	payloads map[string]inputs.Schema
	names    []string
}

// NewCatalog validates declarations into a Catalog.
func NewCatalog(declarations []Declaration) (Catalog, error) {
	if len(declarations) > MaxCommands {
		return Catalog{}, fmt.Errorf("at most %d commands, got %d", MaxCommands, len(declarations))
	}
	catalog := Catalog{payloads: make(map[string]inputs.Schema, len(declarations))}
	for _, declaration := range declarations {
		if !inputs.ValidName(declaration.Name) {
			return Catalog{}, fmt.Errorf("command %q: names start with a letter and hold up to 64 letters, digits or underscores", declaration.Name)
		}
		if _, repeated := catalog.payloads[declaration.Name]; repeated {
			return Catalog{}, fmt.Errorf("command %q is declared twice", declaration.Name)
		}
		payload, err := inputs.NewSchema(declaration.Payload)
		if err != nil {
			return Catalog{}, fmt.Errorf("command %q payload: %w", declaration.Name, err)
		}
		catalog.payloads[declaration.Name] = payload
		catalog.names = append(catalog.names, declaration.Name)
	}
	sort.Strings(catalog.names)
	return catalog, nil
}

// Normalize turns a request's payload, a JSON object of name's declared
// fields or nil, into the full payload in canonical JSON, omitted fields at
// their defaults. An undeclared name wraps ErrUndeclared; an undeclared
// field or invalid value wraps inputs.ErrInvalidValue.
func (c Catalog) Normalize(name string, payload []byte) ([]byte, error) {
	schema, declared := c.payloads[name]
	if !declared {
		return nil, fmt.Errorf("%w: %s", ErrUndeclared, name)
	}
	if payload == nil {
		payload = []byte(`{}`)
	}
	normalized, err := schema.Normalize(payload)
	if err != nil {
		return nil, err
	}
	return []byte(normalized), nil
}

// Declarations returns the validated declarations, payload defaults filled
// in, in name order.
func (c Catalog) Declarations() []Declaration {
	if len(c.names) == 0 {
		return nil
	}
	declarations := make([]Declaration, len(c.names))
	for i, name := range c.names {
		declarations[i] = Declaration{Name: name, Payload: c.payloads[name].Declarations()}
	}
	return declarations
}
