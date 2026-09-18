package sessionlifecycle

import (
	"context"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
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

func (s stubCurrentGameReader) GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*game.Game, error) {
	return &game.Game{UUID: gameUUID, VersionUUID: s.versionUUID, Definition: s.definition}, nil
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
