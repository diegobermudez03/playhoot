package runtime_test

import (
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// numberType and boolType are small local duplicates of the identically
// named helpers in engine/internal/compiler's test package: they can't
// be shared across packages, so per this codebase's convention for
// cross-package test-only fixtures, each side keeps its own minimal
// copy of just what it needs.
func numberType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
}
func boolType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeBool}
}

// withMinimalRootWorkflow adds a trivially valid root workflow to def,
// for tests that exercise other compiler concerns and don't care about
// workflow validation, but must satisfy validateRootWorkflow's now-
// mandatory check to compile without errors.
func withMinimalRootWorkflow(def program.Definition) program.Definition {
	def.Workflows = append(def.Workflows, program.WorkflowDeclaration{
		Name:         "Main",
		ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
		InitialState: "Start",
		States:       []program.WorkflowStateDeclaration{{Name: "Start"}},
	})
	def.RootWorkflow = "Main"
	return def
}
