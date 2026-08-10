package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// Severity classifies a Diagnostic.
type Severity int

const (
	// SeverityError marks a diagnostic that makes the definition
	// uncompilable: Compile does not produce an engine.Program safe to
	// execute when any Diagnostic in its result has this severity.
	SeverityError Severity = iota

	// SeverityWarning marks a diagnostic that does not by itself
	// prevent compilation but flags behavior a creator likely did not
	// intend.
	SeverityWarning

	// SeverityInfo marks a purely informational diagnostic.
	SeverityInfo
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInfo:
		return "info"
	default:
		return "unknown"
	}
}

// Diagnostic reports one compile-time observation about a
// program.Definition.
//
// Path identifies where in the definition the diagnostic applies, in the
// same dotted/bracketed style used by program/gameservice's
// ValidationError and DecodeError (for example
// "$.workflows[0].states[2].transitions[0]"), so tooling built on top of
// program, gameservice, and engineservice can present a consistent
// location format regardless of which layer produced a given diagnostic.
type Diagnostic struct {
	Severity Severity
	Path     string
	Message  string
}

func (d Diagnostic) String() string {
	if d.Path == "" {
		return fmt.Sprintf("%s: %s", d.Severity, d.Message)
	}
	return fmt.Sprintf("%s: %s: %s", d.Severity, d.Path, d.Message)
}

// Diagnostics is an ordered collection of Diagnostic values produced by
// Compile.
//
// Compile collects every diagnostic it can find rather than stopping at
// the first one. A nil or empty Diagnostics means this package's
// compiler found nothing wrong; it is not, by itself, a guarantee that
// the resulting engine.Program is safe to execute in every future sense
// — only that no rule this package's compiler currently checks was
// violated.
type Diagnostics []Diagnostic

// HasErrors reports whether any Diagnostic in ds has SeverityError.
func (ds Diagnostics) HasErrors() bool {
	for _, d := range ds {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Compile validates def and produces its immutable, executable
// representation as an engine.Program.
//
// Compile never panics on an invalid definition and never stops at the
// first problem it finds: it collects every Diagnostic it can and
// returns them alongside the result. When the returned Diagnostics has
// any SeverityError entry, the returned engine.Program must not be
// executed.
//
// Compile does not assume def already passed gameservice.Validate; it
// independently registers def's type namespace, resolves every type
// reference by name, and detects duplicate and undeclared names, because
// it cannot rely on any earlier validation having run.
//
// This version compiles def.Metadata, def.Types (enums, records, unions,
// and new types), def.Functions, def.Resources, def.GlobalState,
// def.Invariants, and def.Workflows, together with the complete pure
// expression language their bodies may use. Projections and views are
// not compiled yet — a workflow presentation only validates that a
// referenced projection or view name exists, not its signature — and
// neither are workflow operations, so a compiled Transition carries only
// its signal pattern, guard, and control; see engine.Transition's doc
// comment. Those, along with the remaining semantic checks described in
// LOGICAL_CONTRACT.md and engine/README.md's "Compiler responsibilities"
// section, are added in later steps.
//
// Compilation order matters here in a way it does not for types and
// functions: a resource's declared Type must be known before functions
// compile, since a function body may reference resources; every
// function must be compiled before any resource Value evaluates, since
// a resource's Value may call one; every resource must be evaluated
// before global state initializers or invariants compile, since both
// may reference resources; and every workflow's declared ResultType must
// be known before any workflow's body compiles, since a child or
// task-group slot's completion signal may reference any workflow's
// (including its own) ResultType.
func Compile(def program.Definition) (engine.Program, Diagnostics) {
	resourceValues := make(map[string]engine.Value)

	c := &compiler{
		definition:              def,
		typeDeclarations:        make(map[string]typeEntry),
		resolvedTypes:           make(map[string]engine.Type),
		resolvingTypes:          make(map[string]bool),
		functionDeclarations:    make(map[string]funcEntry),
		resolvedFunctions:       make(map[string]*engine.Function),
		resolvingFunctions:      make(map[string]bool),
		resourceDeclarations:    make(map[string]resourceEntry),
		resourceTypes:           make(map[string]engine.Type),
		resourceExprs:           make(map[string]engine.Expression),
		resolvedResourceValues:  resourceValues,
		resolvingResourceValues: make(map[string]bool),
		workflowDeclarations:    make(map[string]workflowEntry),
		workflowResultTypes:     make(map[string]engine.Type),
		workflowParameterTypes:  make(map[string][]engine.FieldType),
		compiledProjections:     make(map[string]engine.Projection),
		compiledViews:           make(map[string]engine.View),
	}

	c.registerTypeNamespace()
	types := c.compileTypeDeclarations()

	c.registerResourceNamespace()
	c.buildResourcesRecordType()

	c.registerFunctionNamespace()
	functions := c.compileFunctions()

	c.compileResourceValues()

	p := engine.Program{
		Metadata:  compileMetadata(def.Metadata),
		Types:     types,
		Functions: functions,
		Resources: resourceValues,
	}
	c.evaluateResources(p)

	p.GlobalState = c.compileGlobalState()
	globalType := globalStateRecordType(p.GlobalState)
	p.Invariants = c.compileInvariants(globalType)

	c.compiledQuestions = c.compileQuestions(globalType)
	c.compiledEffects = c.compileEffects()
	p.Questions = c.compiledQuestions
	p.Effects = c.compiledEffects

	// Projections and views must be compiled before any workflow body,
	// since a workflow-level, state-level, or question Presentation
	// validates its Projection's ResultType against its View's
	// ModelType, and its ProjectionArguments against the Projection's
	// own Parameters.
	c.compiledProjections = c.compileProjections(globalType)
	c.compiledViews = c.compileViews()
	p.Projections = c.compiledProjections
	p.Views = c.compiledViews

	c.registerWorkflowNamespace()
	c.buildWorkflowResultTypes()
	c.buildWorkflowParameterTypes()
	c.validateRootWorkflow()
	p.Workflows = c.compileWorkflows(p)
	p.RootWorkflow = def.RootWorkflow

	return p, c.diagnostics
}

func compileMetadata(m program.Metadata) engine.Metadata {
	return engine.Metadata{
		ID:              m.ID,
		Name:            m.Name,
		Description:     m.Description,
		Version:         m.Version,
		LanguageVersion: m.LanguageVersion,
	}
}
