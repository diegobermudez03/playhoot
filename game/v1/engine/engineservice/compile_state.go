package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

// globalScopeRootName is the reserved lexical name a compiled invariant
// (and, in a future step, a guard or operation) uses to reach global
// game state, mirroring program.InvariantDeclaration's documented
// "global" root. It is bound to a RecordValue built fresh from a game
// instance's Snapshot.GlobalState — never a declared name in
// Program.Types.
const globalScopeRootName = "global"

// compileGlobalState compiles c.definition.GlobalState.Fields into
// []engine.StateField, diagnosing an empty or duplicate field name and a
// field whose Initializer does not statically match its declared Type.
//
// A field's Initializer is compiled with only "resources" in scope — it
// cannot reference another global-state field, "global" itself, or
// anything else — so this version does not support one field's initial
// value depending on another's.
func (c *compiler) compileGlobalState() []engine.StateField {
	fields := c.definition.GlobalState.Fields
	path := "$.global_state.fields"
	scope := exprScope{resourcesScopeRootName: c.resourcesType}

	result := make([]engine.StateField, 0, len(fields))
	seen := make(map[string]int, len(fields))
	for i, f := range fields {
		fPath := fmt.Sprintf("%s[%d]", path, i)
		if f.Name == "" {
			c.addf(fPath, "state field has an empty name")
			continue
		}
		if first, ok := seen[f.Name]; ok {
			c.addf(fPath, "duplicate state field name %q (first declared at %s[%d])", f.Name, path, first)
			continue
		}
		seen[f.Name] = i

		t := c.compileTypeReference(f.Type, fPath+".type")
		init, initType := c.compileExpression(f.Initializer, scope, fPath+".initializer")
		if t != nil && initType != nil && !t.Equal(initType) {
			c.addf(fPath+".initializer", "state field %q declares type %s, but its initializer is statically %s", f.Name, describeType(t), describeType(initType))
		}
		result = append(result, engine.StateField{Name: f.Name, Type: t, Initializer: init})
	}
	return result
}

// compileInvariants compiles c.definition.Invariants into
// []engine.Invariant, diagnosing a duplicate name and a Condition that
// is not statically bool. Every invariant is evaluated unconditionally
// (never looked up by name), so a duplicate name does not affect
// evaluation — it is still diagnosed, per program.InvariantDeclaration's
// documented expectation that "the future compiler" report it.
func (c *compiler) compileInvariants(globalType engine.RecordType) []engine.Invariant {
	scope := exprScope{resourcesScopeRootName: c.resourcesType, globalScopeRootName: globalType}

	seen := make(map[string]int, len(c.definition.Invariants))
	result := make([]engine.Invariant, 0, len(c.definition.Invariants))
	for i, inv := range c.definition.Invariants {
		path := fmt.Sprintf("$.invariants[%d]", i)
		if first, ok := seen[inv.Name]; ok && inv.Name != "" {
			c.addf(path, "duplicate invariant name %q (first declared at $.invariants[%d])", inv.Name, first)
		}
		if inv.Name != "" {
			seen[inv.Name] = i
		}

		cond, condType := c.compileExpression(inv.Condition, scope, path+".condition")
		if condType != nil {
			if _, ok := condType.(engine.BoolType); !ok {
				c.addf(path+".condition", "invariant %q must be statically bool, but it is %s", inv.Name, describeType(condType))
			}
		}
		result = append(result, engine.Invariant{Name: inv.Name, Condition: cond})
	}
	return result
}

// globalStateRecordType derives the RecordType shape of global state
// from its compiled fields, for use as the "global" scope root when
// compiling invariants.
func globalStateRecordType(fields []engine.StateField) engine.RecordType {
	fieldTypes := make([]engine.FieldType, len(fields))
	for i, f := range fields {
		fieldTypes[i] = engine.FieldType{Name: f.Name, Type: f.Type}
	}
	return engine.RecordType{Name: globalScopeRootName, Fields: fieldTypes}
}
