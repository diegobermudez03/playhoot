package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// registerFunctionNamespace walks c.definition.Functions in source
// order, diagnosing an empty name, a name colliding with a built-in
// function, and a duplicate name, and registers the first occurrence of
// every other name into c.functionDeclarations.
func (c *compiler) registerFunctionNamespace() {
	seen := make(map[string]int, len(c.definition.Functions))
	for i, f := range c.definition.Functions {
		path := fmt.Sprintf("$.functions[%d]", i)

		if f.Name == "" {
			c.addf(path, "function declaration has an empty name")
			continue
		}
		if isBuiltinFunction(f.Name) {
			c.addf(path, "function name %q conflicts with a built-in function", f.Name)
			continue
		}
		if first, ok := seen[f.Name]; ok {
			c.addf(path, "duplicate function name %q (first declared at $.functions[%d])", f.Name, first)
			continue
		}

		seen[f.Name] = i
		c.functionDeclarations[f.Name] = funcEntry{decl: f, path: path, index: i}
	}
}

// compileFunctions compiles every registered function name, in source
// order, mirroring compileTypeDeclarations.
func (c *compiler) compileFunctions() map[string]engine.Function {
	result := make(map[string]engine.Function, len(c.functionDeclarations))
	for i, f := range c.definition.Functions {
		if f.Name == "" {
			continue
		}
		entry, registered := c.functionDeclarations[f.Name]
		if !registered || entry.index != i {
			continue
		}
		if fn := c.resolveFunction(f.Name); fn != nil {
			result[f.Name] = *fn
		}
	}
	return result
}

// resolveFunction returns the compiled *engine.Function for the
// registered function name, compiling and memoizing it on first use. It
// assumes name is already registered in c.functionDeclarations; callers
// resolving a call to a function by name must check existence, and
// check c.resolvingFunctions for recursion, before calling this.
func (c *compiler) resolveFunction(name string) *engine.Function {
	if fn, ok := c.resolvedFunctions[name]; ok {
		return fn
	}
	entry := c.functionDeclarations[name]

	c.resolvingFunctions[name] = true
	fn := c.compileFunctionDeclaration(entry.decl, entry.path)
	delete(c.resolvingFunctions, name)

	c.resolvedFunctions[name] = fn
	return fn
}

// compileFunctionDeclaration compiles d's parameters and result type,
// then compiles Body in a scope containing exactly those parameters
// plus the reserved "resources" root — per
// program.FunctionDeclaration's documented scope restriction, a
// function body sees only its own parameters, resources, other
// functions, and built-ins, never global, local, signal, or any other
// execution-specific root.
func (c *compiler) compileFunctionDeclaration(d program.FunctionDeclaration, path string) *engine.Function {
	scope := exprScope{resourcesScopeRootName: c.resourcesType}
	params := make([]engine.FieldType, 0, len(d.Parameters))
	seen := make(map[string]int, len(d.Parameters))
	paramsPath := path + ".parameters"
	for i, p := range d.Parameters {
		pPath := fmt.Sprintf("%s[%d]", paramsPath, i)
		if p.Name == "" {
			c.addf(pPath, "parameter has an empty name")
			continue
		}
		if first, ok := seen[p.Name]; ok {
			c.addf(pPath, "duplicate parameter name %q (first declared at %s[%d])", p.Name, paramsPath, first)
			continue
		}
		seen[p.Name] = i
		t := c.compileTypeReference(p.Type, pPath+".type")
		scope[p.Name] = t
		params = append(params, engine.FieldType{Name: p.Name, Type: t})
	}

	resultType := c.compileTypeReference(d.ResultType, path+".result_type")
	body, bodyType := c.compileExpression(d.Body, scope, path+".body")

	if resultType != nil && bodyType != nil && !resultType.Equal(bodyType) {
		c.addf(path+".body", "function %q declares result type %s, but its body is statically %s", d.Name, describeType(resultType), describeType(bodyType))
	}

	return &engine.Function{Name: d.Name, Parameters: params, ResultType: resultType, Body: body}
}
