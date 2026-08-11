package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// builtinSignature declares one built-in pure function's fixed
// parameter names/types and result type, for every built-in whose
// arguments are not polymorphic. length is the one exception (it
// accepts a list, a map, or a string) and is checked separately in
// compileLengthCall.
type builtinSignature struct {
	Parameters []engine.FieldType
	Result     engine.Type
}

// builtinSignatures is this engine's minimum pure built-in catalog.
// program/list_query_expression.go names length, sum, minimum, maximum,
// and format as illustrative examples of built-ins that remain ordinary
// CallExpressions; this version implements length, min, max, and abs.
var builtinSignatures = map[string]builtinSignature{
	"min": {
		Parameters: []engine.FieldType{{Name: "a", Type: engine.NumberType{}}, {Name: "b", Type: engine.NumberType{}}},
		Result:     engine.NumberType{},
	},
	"max": {
		Parameters: []engine.FieldType{{Name: "a", Type: engine.NumberType{}}, {Name: "b", Type: engine.NumberType{}}},
		Result:     engine.NumberType{},
	},
	"abs": {
		Parameters: []engine.FieldType{{Name: "value", Type: engine.NumberType{}}},
		Result:     engine.NumberType{},
	},
}

// isBuiltinFunction reports whether name is reserved by the built-in
// catalog, so a user-declared function cannot collide with one.
func isBuiltinFunction(name string) bool {
	if name == "length" {
		return true
	}
	_, ok := builtinSignatures[name]
	return ok
}

func (c *compiler) compileCallExpression(e program.CallExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	if e.Function == "" {
		c.addf(path+".function", "call expression has an empty function name")
		return nil, nil
	}

	args, argTypes, argsOK := c.compileCallArguments(e.Arguments, scope, path)

	if e.Function == "length" {
		return c.compileLengthCall(args, argTypes, argsOK, path)
	}

	if sig, ok := builtinSignatures[e.Function]; ok {
		if !argsOK || !c.checkCallArguments(sig.Parameters, args, argTypes, path) {
			return nil, nil
		}
		return engine.CallExpression{Function: e.Function, Arguments: args}, sig.Result
	}

	if _, ok := c.functionDeclarations[e.Function]; !ok {
		c.addf(path+".function", "call to undeclared function %q", e.Function)
		return nil, nil
	}
	if c.resolvingFunctions[e.Function] {
		c.addf(path, "function %q is called recursively (directly or indirectly), which this language does not support", e.Function)
		return nil, nil
	}
	fn := c.resolveFunction(e.Function)
	if fn == nil || !argsOK || !c.checkCallArguments(fn.Parameters, args, argTypes, path) {
		return nil, nil
	}
	return engine.CallExpression{Function: e.Function, Arguments: args}, fn.ResultType
}

// compileCallArguments compiles every argument, diagnosing an empty or
// duplicate argument name. It always returns one engine.CallArgument per
// input argument entry (even one that failed to compile, so the caller
// can still report shape-level problems), plus ok == false if any
// argument's value did not compile to a known type.
func (c *compiler) compileCallArguments(rawArgs []program.CallArgument, scope exprScope, path string) ([]engine.CallArgument, map[string]engine.Type, bool) {
	args := make([]engine.CallArgument, 0, len(rawArgs))
	types := make(map[string]engine.Type, len(rawArgs))
	seen := make(map[string]int, len(rawArgs))
	ok := true
	for i, a := range rawArgs {
		argPath := fmt.Sprintf("%s.arguments[%d]", path, i)
		if a.Name == "" {
			c.addf(argPath+".name", "call argument has an empty name")
			ok = false
			continue
		}
		if first, dup := seen[a.Name]; dup {
			c.addf(argPath, "duplicate argument %q (first provided at %s.arguments[%d])", a.Name, path, first)
			ok = false
			continue
		}
		seen[a.Name] = i

		val, typ := c.compileExpression(a.Value, scope, argPath+".value")
		if typ == nil {
			ok = false
		}
		args = append(args, engine.CallArgument{Name: a.Name, Value: val})
		types[a.Name] = typ
	}
	return args, types, ok
}

// checkCallArguments reports whether args exactly matches params: every
// parameter provided exactly once with a statically compatible type, and
// no unknown argument name.
func (c *compiler) checkCallArguments(params []engine.FieldType, args []engine.CallArgument, argTypes map[string]engine.Type, path string) bool {
	ok := true
	for _, p := range params {
		t, has := argTypes[p.Name]
		if !has {
			c.addf(path, "missing required argument %q", p.Name)
			ok = false
			continue
		}
		if t != nil && !t.Equal(p.Type) {
			c.addf(path, "argument %q is statically %s, but %s is required", p.Name, describeType(t), describeType(p.Type))
			ok = false
		}
	}

	allowed := make(map[string]bool, len(params))
	for _, p := range params {
		allowed[p.Name] = true
	}
	for _, a := range args {
		if !allowed[a.Name] {
			c.addf(path, "unknown argument %q", a.Name)
			ok = false
		}
	}
	return ok
}

// compileLengthCall type-checks the one built-in whose argument type is
// polymorphic: length accepts a list, a map, or a string.
func (c *compiler) compileLengthCall(args []engine.CallArgument, argTypes map[string]engine.Type, argsOK bool, path string) (engine.Expression, engine.Type) {
	if len(args) != 1 || args[0].Name != "value" {
		c.addf(path, `length requires exactly one argument named "value"`)
		return nil, nil
	}
	if !argsOK {
		return nil, nil
	}
	switch argTypes["value"].(type) {
	case engine.ListType, engine.MapType, engine.StringType:
	default:
		c.addf(path, "length requires a list, map, or string argument, but %q is statically %s", "value", describeType(argTypes["value"]))
		return nil, nil
	}
	return engine.CallExpression{Function: "length", Arguments: args}, engine.NumberType{}
}
