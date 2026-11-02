// Package gamemanifest turns the manifest of a game version into the rules
// the Runtime admits its inputs and commands with. The live Session
// controller and the simulation share it.
package gamemanifest

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/play/internal/business/commands"
	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// Rules are what the Runtime knows about a game version's players, inputs
// and commands.
type Rules struct {
	MinPlayers int
	MaxPlayers int

	// Inputs are what the game's SAMPLEs carry; empty means it takes none.
	Inputs inputs.Schema

	// Commands are the COMMANDs the game accepts and their payloads.
	Commands commands.Catalog

	// Update means the game implements update(), run every boundary.
	Update bool

	// Rejoin means the game implements playerRejoined().
	Rejoin bool
}

// Compile reads loaded into Rules; an invalid declaration is an error.
func Compile(loaded sessionpkg.RuntimeManifest) (Rules, error) {
	schema, err := inputs.NewSchema(Inputs(loaded.Inputs))
	if err != nil {
		return Rules{}, fmt.Errorf("reading the manifest's inputs: %w", err)
	}
	declared := make([]commands.Declaration, len(loaded.Commands))
	for i, command := range loaded.Commands {
		declared[i] = commands.Declaration{Name: command.Name, Payload: Inputs(command.Payload)}
	}
	catalog, err := commands.NewCatalog(declared)
	if err != nil {
		return Rules{}, fmt.Errorf("reading the manifest's commands: %w", err)
	}
	return Rules{
		MinPlayers: loaded.MinPlayers,
		MaxPlayers: loaded.MaxPlayers,
		Inputs:     schema,
		Commands:   catalog,
		Update:     loaded.Update,
		Rejoin:     loaded.Rejoin,
	}, nil
}

// Inputs is a manifest's input declarations as the inputs package takes them.
func Inputs(declared []sessionpkg.RuntimeInput) []inputs.Declaration {
	declarations := make([]inputs.Declaration, len(declared))
	for i, input := range declared {
		declarations[i] = inputs.Declaration{
			Name: input.Name, Type: inputs.Type(input.Type), Nullable: input.Nullable,
			Min: input.Min, Max: input.Max, Values: input.Values, Default: input.Default,
		}
	}
	return declarations
}
