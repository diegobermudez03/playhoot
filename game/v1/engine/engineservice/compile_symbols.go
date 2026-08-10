package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// compiler holds the working state of one Compile call: the source
// definition being compiled, the diagnostics collected so far, and the
// namespaces and memoization tables built while compiling it.
//
// A compiler is used for exactly one Compile call and discarded; it is
// not safe to reuse or share across calls.
type compiler struct {
	definition program.Definition

	diagnostics Diagnostics

	// typeDeclarations registers the type namespace: every declared
	// type name that survived duplicate detection, together with its
	// source declaration and source-model path. Only the first
	// occurrence of a duplicate name is registered.
	typeDeclarations map[string]typeEntry

	// resolvedTypes memoizes the compiled engine.Type for each name in
	// typeDeclarations, so a name referenced from more than one place
	// is only compiled once.
	resolvedTypes map[string]engine.Type

	// resolvingTypes tracks which names are currently being compiled,
	// so a type that refers back to itself — directly, or through
	// another type, a list, a map, or an optional — can be detected and
	// diagnosed instead of recursing forever.
	resolvingTypes map[string]bool

	// functionDeclarations registers the function namespace, the same
	// way typeDeclarations does for types: every declared function name
	// that survived duplicate and built-in-collision detection.
	functionDeclarations map[string]funcEntry

	// resolvedFunctions memoizes the compiled *engine.Function for each
	// name in functionDeclarations.
	resolvedFunctions map[string]*engine.Function

	// resolvingFunctions tracks which function names are currently
	// being compiled, so a call graph that calls back into a function
	// already being compiled — directly, or through another function —
	// can be diagnosed as recursive instead of recursing forever. This
	// initial language disallows all function recursion, unlike named
	// types, which only reject an actual cycle.
	resolvingFunctions map[string]bool

	// resourcesType is the synthetic RecordType shape of the "resources"
	// scope root, built once (from every registered resource's declared
	// Type, before any resource Value is evaluated) and reused whenever
	// a function body, a resource Value, a global-state initializer, or
	// an invariant condition is compiled.
	resourcesType engine.RecordType

	// resourceDeclarations registers the resource namespace, the same
	// way typeDeclarations does for types.
	resourceDeclarations map[string]resourceEntry

	// resourceTypes memoizes each registered resource's compiled Type,
	// computed once from its own declaration without evaluating
	// anything — this is what lets the "resources" scope root exist
	// before any resource Value is evaluated.
	resourceTypes map[string]engine.Type

	// resourceExprs memoizes each registered resource's compiled Value
	// expression.
	resourceExprs map[string]engine.Expression

	// resolvedResourceValues memoizes each resource's evaluated Value,
	// filled in dependency order by resolveResourceValue. This map is
	// also the live backing store the evaluator reads through the
	// "resources" scope root while evaluating another resource's Value,
	// a called function's body, or (later) global state initializers
	// and invariants — see evaluate.go's withResources.
	resolvedResourceValues map[string]engine.Value

	// resolvingResourceValues tracks which resource names are currently
	// being evaluated, so a dependency cycle — direct, or through
	// another resource or a called function — can be diagnosed instead
	// of recursing forever.
	resolvingResourceValues map[string]bool

	// workflowDeclarations registers the workflow namespace, the same
	// way typeDeclarations does for types.
	workflowDeclarations map[string]workflowEntry

	// workflowResultTypes memoizes each registered workflow's compiled
	// ResultType, computed once (in registerWorkflowNamespace's second
	// pass, before any workflow's body compiles) from its own
	// declaration without compiling the rest of it — this is what lets
	// one workflow's child/task-group slot resolve another (or its own)
	// ResultType for a child/task-group completion signal's schema
	// without needing that workflow's full body compiled first, and
	// without the recursion risk a named type or function has: a
	// workflow's ResultType never depends on another workflow's body.
	workflowResultTypes map[string]engine.Type

	// workflowParameterTypes memoizes each registered workflow's
	// compiled Parameters, computed once (in buildWorkflowParameterTypes,
	// before any workflow's body compiles) for the same reason
	// workflowResultTypes is: a SpawnChildWorkflowOperation targeting a
	// child slot must validate its Arguments against that slot's
	// declared workflow's parameters without needing that workflow's
	// full body compiled first, and without compiling the same
	// parameter declarations — and diagnosing them — a second time.
	workflowParameterTypes map[string][]engine.FieldType

	// compiledQuestions and compiledEffects hold every compiled
	// program.QuestionDeclaration and program.EffectDeclaration, keyed
	// by declared name, computed once before any workflow compiles —
	// an OpenQuestionOperation or EmitEffectOperation validates its
	// Arguments against these.
	compiledQuestions map[string]engine.Question
	compiledEffects   map[string]engine.Effect
}

// typeEntry is the registered namespace entry for one declared type
// name.
type typeEntry struct {
	decl  program.TypeDeclaration
	path  string
	index int
}

// funcEntry is the registered namespace entry for one declared function
// name.
type funcEntry struct {
	decl  program.FunctionDeclaration
	path  string
	index int
}

// exprScope is the compile-time symbol table in effect while compiling
// one Expression: the set of lexical names visible at this point and
// their statically known Type. It is unrelated to engine.Scope, the
// runtime binding of names to Value used by evaluation — exprScope only
// exists during compilation, to validate references and infer types.
type exprScope map[string]engine.Type

// clone returns a copy of s, so that adding a binding for a nested scope
// (a match case, a list query iteration) never affects s itself or any
// other scope derived from it.
func (s exprScope) clone() exprScope {
	n := make(exprScope, len(s)+1)
	for k, v := range s {
		n[k] = v
	}
	return n
}

// addf records one SeverityError Diagnostic at path.
func (c *compiler) addf(path, format string, args ...any) {
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Severity: SeverityError,
		Path:     path,
		Message:  fmt.Sprintf(format, args...),
	})
}

// registerTypeNamespace walks c.definition.Types in source order,
// diagnosing an empty or unsupported declaration name and a duplicate
// name, and registers the first occurrence of every other name into
// c.typeDeclarations.
//
// Every declared type — enum, record, union, and new type — shares one
// namespace, the same way program itself treats $.types as a single
// namespace regardless of which kind of TypeDeclaration is involved.
func (c *compiler) registerTypeNamespace() {
	seen := make(map[string]int, len(c.definition.Types))
	for i, t := range c.definition.Types {
		path := fmt.Sprintf("$.types[%d]", i)

		name, ok := typeDeclarationName(t)
		if !ok {
			c.addf(path, "missing or unsupported type declaration")
			continue
		}
		if name == "" {
			c.addf(path, "type declaration has an empty name")
			continue
		}
		if first, ok := seen[name]; ok {
			c.addf(path, "duplicate type name %q (first declared at $.types[%d])", name, first)
			continue
		}

		seen[name] = i
		c.typeDeclarations[name] = typeEntry{decl: t, path: path, index: i}
	}
}

// typeDeclarationName returns t's declared name, or ok == false if t is
// nil or not one of program's closed TypeDeclaration variants.
func typeDeclarationName(t program.TypeDeclaration) (string, bool) {
	switch tt := t.(type) {
	case program.EnumTypeDeclaration:
		return tt.Name, true
	case program.RecordTypeDeclaration:
		return tt.Name, true
	case program.UnionTypeDeclaration:
		return tt.Name, true
	case program.NewTypeDeclaration:
		return tt.Name, true
	default:
		return "", false
	}
}
