package sessionlifecycle

import (
	"context"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/management"
)

// compilableDefinitionForTest is a minimal Game Language definition that
// compiles successfully, reused by integration tests that need Create to
// succeed against a real engineservice.Compile call.
func compilableDefinitionForTest(playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "parques", Name: "Parques"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Max: playersMax},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				States:       []program.WorkflowStateDeclaration{{Name: "Start"}},
			},
		},
	}
}

// uncompilableDefinitionForTest is a Game Language definition that fails
// engineservice.Compile, reused by integration tests that need Create to be
// rejected for a broken definition.
func uncompilableDefinitionForTest() program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "broken", Name: "Broken"},
		RootWorkflow: "does-not-exist",
	}
}

// stubCurrentGameReader satisfies gameCurrentVersionReader with a fixed
// playable Game, so integration tests can exercise the real public
// Manager.Create end to end without depending on Game Management.
type stubCurrentGameReader struct {
	gameUUID    string
	versionUUID string
	definition  program.Definition
}

func (s stubCurrentGameReader) GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*management.Game, error) {
	return &management.Game{UUID: gameUUID, VersionUUID: s.versionUUID, Definition: s.definition}, nil
}

// stubPinnedGameReader satisfies gamePinnedDefinitionReader with a fixed
// pinned Definition, so integration tests can exercise the real public
// Manager.Join end to end without depending on Game Management.
type stubPinnedGameReader struct {
	playersMax int
}

func (s stubPinnedGameReader) GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error) {
	return &program.Definition{Players: program.PlayerPolicy{Max: s.playersMax}}, nil
}

// startableDefinition builds a real, engineservice.Compile-able Definition
// declaring the accepted `players: list<user>` root roster parameter
// (game/README.md's Accepted Game Language Root Roster Contract) and a
// root workflow that actually reacts to WorkflowStarted with StayControl -
// unlike testutil_test.go's other fixtures (compilableDefinitionForTest,
// stubPinnedGameReader's minimal stub), which never execute past compile.
// Start's integration tests need a Definition that both compiles and
// executes its first RuntimeTurn successfully.
func startableDefinition(playersMin, playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "startable", Name: "Startable"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{Name: "Started", Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.StayControl{}},
						},
					},
				},
			},
		},
	}
}

// nonStartableDefinition builds a Definition that compiles successfully
// (so Create could pin it) but whose root workflow declares no transition
// at all for WorkflowStarted - Start's mandatory first Step call is then an
// outright rejection, forcing the pre-first-Turn RUNTIME_EXECUTION_FAILED
// fatal path (WORK-0003's Fatal-Path Classification) deterministically, for
// tests that need to force that path against a real database.
func nonStartableDefinition(playersMin, playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "non-startable", Name: "NonStartable"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				States:       []program.WorkflowStateDeclaration{{Name: "Start"}},
			},
		},
	}
}

// stubStartPinnedGameReader satisfies gamePinnedDefinitionReader with a
// fixed, caller-supplied Definition, so Start's integration tests can
// exercise real engineservice.Compile/NewSnapshot/Step execution end to
// end against whichever fixture (startableDefinition/
// nonStartableDefinition) a given test case needs.
type stubStartPinnedGameReader struct {
	definition program.Definition
}

func (s stubStartPinnedGameReader) GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error) {
	return &s.definition, nil
}
