package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// resourceEntry is the registered namespace entry for one declared
// resource name.
type resourceEntry struct {
	decl  program.ResourceDeclaration
	path  string
	index int
}

// resourcesScopeRootName is the reserved lexical name a compiled
// expression uses to reach immutable resources, mirroring
// program.ResourceDeclaration's and program.FunctionDeclaration's own
// documented "resources" root. It is bound in scope to a synthetic
// RecordType (at compile time) or RecordValue (at evaluation time) with
// one field per declared resource; that RecordType/RecordValue is never
// itself a declared named type or added to Program.Types. Aliases
// engine.ResourcesScopeRootName so the compiler and the runtime (a
// separate internal package) agree on exactly this name.
const resourcesScopeRootName = engine.ResourcesScopeRootName

// registerResourceNamespace walks c.definition.Resources in source
// order, diagnosing an empty or duplicate name, and registers the first
// occurrence of every other name into c.resourceDeclarations. Resources
// have their own namespace, independent of types and functions.
func (c *compiler) registerResourceNamespace() {
	seen := make(map[string]int, len(c.definition.Resources))
	for i, r := range c.definition.Resources {
		path := fmt.Sprintf("$.resources[%d]", i)
		if r.Name == "" {
			c.addf(path, "resource declaration has an empty name")
			continue
		}
		if first, ok := seen[r.Name]; ok {
			c.addf(path, "duplicate resource name %q (first declared at $.resources[%d])", r.Name, first)
			continue
		}
		seen[r.Name] = i
		c.resourceDeclarations[r.Name] = resourceEntry{decl: r, path: path, index: i}
	}
}

// buildResourcesRecordType compiles every registered resource's declared
// Type — never its Value — and stores the result as c.resourcesType, so
// the "resources" scope root exists before any resource Value, function
// body, global state initializer, or invariant is compiled.
func (c *compiler) buildResourcesRecordType() {
	fields := make([]engine.FieldType, 0, len(c.resourceDeclarations))
	for i, r := range c.definition.Resources {
		entry, registered := c.resourceDeclarations[r.Name]
		if !registered || entry.index != i {
			continue
		}
		t := c.compileTypeReference(r.Type, entry.path+".type")
		c.resourceTypes[r.Name] = t
		fields = append(fields, engine.FieldType{Name: r.Name, Type: t})
	}
	c.resourcesType = engine.RecordType{Name: resourcesScopeRootName, Fields: fields}
}

// compileResourceValues compiles every registered resource's Value
// expression against c.resourcesType, checking it statically matches the
// resource's declared Type. It does not evaluate anything; evaluation,
// in dependency order, is resolveResourceValue's job.
func (c *compiler) compileResourceValues() {
	scope := exprScope{resourcesScopeRootName: c.resourcesType}
	for i, r := range c.definition.Resources {
		entry, registered := c.resourceDeclarations[r.Name]
		if !registered || entry.index != i {
			continue
		}
		expr, exprType := c.compileExpression(r.Value, scope, entry.path+".value")
		if exprType != nil {
			declaredType := c.resourceTypes[r.Name]
			if declaredType != nil && !declaredType.Equal(exprType) {
				c.addf(entry.path+".value", "resource %q declares type %s, but its value is statically %s", r.Name, describeType(declaredType), describeType(exprType))
			}
		}
		c.resourceExprs[r.Name] = expr
	}
}

// evaluateResources evaluates every registered resource's compiled
// Value, in dependency order, into p.Resources — which must already be
// the same map c.resolvedResourceValues writes into, so the evaluator's
// "resources" scope root (see evaluate.go's withResources) sees each
// dependency's finished Value as soon as it is resolved.
func (c *compiler) evaluateResources(p engine.Program) {
	for i, r := range c.definition.Resources {
		entry, registered := c.resourceDeclarations[r.Name]
		if !registered || entry.index != i {
			continue
		}
		c.resolveResourceValue(r.Name, p)
	}
}

// resolveResourceValue returns the evaluated Value for the registered
// resource name and whether resolution succeeded, evaluating and
// memoizing it — after first resolving every resource it depends on,
// directly or through a called function — on first use.
//
// If any dependency fails to resolve (most often because it is part of
// a cycle involving name), resolveResourceValue reports ok == false and
// never evaluates name's own Value expression at all: doing so would
// read an unresolved dependency's entry from the shared "resources"
// scope root, which does not indicate a legitimate runtime condition —
// see evaluate.go's FieldExpression case — so this must be caught here,
// before Evaluate ever runs.
func (c *compiler) resolveResourceValue(name string, p engine.Program) (engine.Value, bool) {
	if v, ok := c.resolvedResourceValues[name]; ok {
		return v, true
	}
	entry, ok := c.resourceDeclarations[name]
	if !ok {
		return nil, false
	}
	if c.resolvingResourceValues[name] {
		c.addf(entry.path, "resource %q depends on itself (directly, through another resource, or through a called function's own resource references), which this compiler does not support", name)
		return nil, false
	}

	c.resolvingResourceValues[name] = true
	deps := map[string]bool{}
	collectExpressionResourceRefs(c.resourceExprs[name], p.Functions, deps, map[string]bool{})
	depsOK := true
	for dep := range deps {
		if _, declared := c.resourceDeclarations[dep]; declared {
			if _, ok := c.resolveResourceValue(dep, p); !ok {
				depsOK = false
			}
		}
	}
	delete(c.resolvingResourceValues, name)
	if !depsOK {
		return nil, false
	}

	v, err := runtime.Evaluate(p, c.resourceExprs[name], engine.Scope{})
	if err != nil {
		c.addf(entry.path+".value", "resource %q failed to evaluate: %s", name, err)
		return nil, false
	}

	c.resolvedResourceValues[name] = v
	return v, true
}

// collectExpressionResourceRefs walks expr and adds to refs the name of
// every resource it reaches through the "resources" scope root —
// directly, or transitively through a call to a user-declared function,
// expanding each called function's own body at most once via
// visitedFunctions. Functions are already known to be free of recursion
// by the time this runs, so this always terminates.
func collectExpressionResourceRefs(expr engine.Expression, functions map[string]engine.Function, refs map[string]bool, visitedFunctions map[string]bool) {
	switch x := expr.(type) {
	case nil, engine.UnitLiteralExpression, engine.BoolLiteralExpression, engine.NumberLiteralExpression, engine.StringLiteralExpression,
		engine.EnumValueExpression, engine.ReferenceExpression:
		return

	case engine.OptionalNoneExpression:
		return
	case engine.OptionalSomeExpression:
		collectExpressionResourceRefs(x.Value, functions, refs, visitedFunctions)

	case engine.ListExpression:
		for _, el := range x.Elements {
			collectExpressionResourceRefs(el, functions, refs, visitedFunctions)
		}
	case engine.MapExpression:
		for _, entry := range x.Entries {
			collectExpressionResourceRefs(entry.Key, functions, refs, visitedFunctions)
			collectExpressionResourceRefs(entry.Value, functions, refs, visitedFunctions)
		}
	case engine.RecordExpression:
		for _, f := range x.Fields {
			collectExpressionResourceRefs(f.Value, functions, refs, visitedFunctions)
		}
	case engine.UnionExpression:
		for _, f := range x.Fields {
			collectExpressionResourceRefs(f.Value, functions, refs, visitedFunctions)
		}
	case engine.NewTypeExpression:
		collectExpressionResourceRefs(x.Value, functions, refs, visitedFunctions)

	case engine.FieldExpression:
		if ref, ok := x.Target.(engine.ReferenceExpression); ok && ref.Name == resourcesScopeRootName {
			refs[x.Field] = true
		}
		collectExpressionResourceRefs(x.Target, functions, refs, visitedFunctions)

	case engine.IndexExpression:
		collectExpressionResourceRefs(x.Target, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Index, functions, refs, visitedFunctions)

	case engine.UnaryExpression:
		collectExpressionResourceRefs(x.Operand, functions, refs, visitedFunctions)

	case engine.BinaryExpression:
		collectExpressionResourceRefs(x.Left, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Right, functions, refs, visitedFunctions)

	case engine.ConditionalExpression:
		collectExpressionResourceRefs(x.Condition, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Then, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Else, functions, refs, visitedFunctions)

	case engine.CallExpression:
		for _, a := range x.Arguments {
			collectExpressionResourceRefs(a.Value, functions, refs, visitedFunctions)
		}
		if fn, ok := functions[x.Function]; ok && !visitedFunctions[x.Function] {
			visitedFunctions[x.Function] = true
			collectExpressionResourceRefs(fn.Body, functions, refs, visitedFunctions)
		}

	case engine.MatchExpression:
		collectExpressionResourceRefs(x.Value, functions, refs, visitedFunctions)
		for _, cs := range x.Cases {
			collectExpressionResourceRefs(cs.Result, functions, refs, visitedFunctions)
		}

	case engine.ListMapExpression:
		collectExpressionResourceRefs(x.Collection, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Result, functions, refs, visitedFunctions)
	case engine.ListFilterExpression:
		collectExpressionResourceRefs(x.Collection, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Predicate, functions, refs, visitedFunctions)
	case engine.ListFlatMapExpression:
		collectExpressionResourceRefs(x.Collection, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Result, functions, refs, visitedFunctions)
	case engine.ListAnyExpression:
		collectExpressionResourceRefs(x.Collection, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Predicate, functions, refs, visitedFunctions)
	case engine.ListAllExpression:
		collectExpressionResourceRefs(x.Collection, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Predicate, functions, refs, visitedFunctions)
	case engine.ListCountExpression:
		collectExpressionResourceRefs(x.Collection, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Predicate, functions, refs, visitedFunctions)
	case engine.ListFirstExpression:
		collectExpressionResourceRefs(x.Collection, functions, refs, visitedFunctions)
		collectExpressionResourceRefs(x.Predicate, functions, refs, visitedFunctions)
	}
}
