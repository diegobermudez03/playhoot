package engine

// Program is the compiled, immutable representation of one game
// version.
//
// Program is plain data, the same way program.Definition is: this
// package enforces no invariant on it, and nothing stops a caller from
// constructing one by hand. The guarantee that a Program is actually
// safe to execute belongs to whichever service produced it (see
// engineservice.Compile) — engine itself only describes the shape of a
// compiled game version, intended to be immutable and safe to share once
// obtained from a trusted source.
//
// Program deliberately does not retain the program.Definition it was
// compiled from: engine does not import program at all (see doc.go), and
// a caller that still needs the original Definition already has it,
// since it is the caller who passes it to engineservice.Compile.
//
// Program is intentionally incomplete and will continue to grow:
// compiled workflows and executable instructions are added as
// compilation semantics are implemented.
type Program struct {
	// Metadata is the compiled identity and versioning information of
	// this game version, carried over unchanged from
	// program.Definition.Metadata.
	Metadata Metadata

	// Types holds every named type declared by the compiled
	// program.Definition — enums, records, unions, and new types —
	// keyed by declared name and fully resolved: a field whose source
	// type reference names another declared type holds that other
	// type's own compiled Type value directly, not a further name to
	// look up.
	//
	// Types does not yet support a named type that, directly or
	// indirectly (including through a list, map, or optional), refers
	// back to itself; engineservice.Compile reports that as a
	// SeverityError diagnostic instead of compiling it.
	Types map[string]Type

	// Functions holds every user-declared pure function of the compiled
	// program.Definition, keyed by declared name. A function's Body
	// never sees another function's scope; recursion — direct,
	// indirect, or through a cycle of several functions — is reported
	// as a SeverityError diagnostic instead of being compiled.
	Functions map[string]Function

	// Resources holds every immutable, evaluated resource value of the
	// compiled program.Definition, keyed by declared name. Unlike
	// Types and Functions, a Resource has no further compiled shape to
	// keep: its Value is evaluated once, during compilation, and this
	// is that evaluated result. Evaluating an expression through
	// engineservice.Evaluate makes every entry here available through
	// the reserved lexical name "resources" — see evaluate.go's
	// withResources.
	//
	// Resources does not yet support a resource that depends on itself
	// — directly, through another resource, or through a called
	// function's own resource references; engineservice.Compile reports
	// that as a SeverityError diagnostic instead of evaluating it.
	Resources map[string]Value

	// GlobalState holds the compiled fields of
	// program.Definition.GlobalState, in declaration order.
	// engineservice.NewSnapshot evaluates these once per new game
	// instance to build that instance's initial global state.
	GlobalState []StateField

	// Invariants holds every compiled program.InvariantDeclaration.
	// engineservice.NewSnapshot evaluates every one of these against a
	// new game instance's initial global state and rejects
	// initialization atomically if any is false or fails to evaluate.
	Invariants []Invariant

	// Questions holds every compiled program.QuestionDeclaration, keyed
	// by declared name. A workflow's OpenQuestionOperation validates its
	// Arguments against the named entry's Parameters at compile time;
	// engineservice.Step evaluates its Validation, if any, against a
	// submitted answer before ever producing a
	// QuestionAnsweredSignalSource signal.
	Questions map[string]Question

	// Effects holds every compiled program.EffectDeclaration, keyed by
	// declared name. A workflow's EmitEffectOperation validates its
	// Arguments against the named entry's Parameters at compile time.
	Effects map[string]Effect

	// Projections holds every compiled program.ProjectionDeclaration,
	// keyed by declared name. A Presentation or QuestionPresentation
	// validates its ProjectionArguments against the named entry's
	// Parameters at compile time; engineservice evaluates Body, per
	// viewer, only against an already-committed snapshot.
	Projections map[string]Projection

	// Views holds every compiled program.ViewDeclaration, keyed by
	// declared name. A Presentation or QuestionPresentation validates
	// its referenced Projection's ResultType is assignable to the named
	// entry's ModelType at compile time. The engine never mounts or
	// renders a View; a client fetches it once, by name, and constructs
	// its interface from that plus the Model an ActivatePresentationOutput
	// or UpdatePresentationOutput carries.
	Views map[string]View

	// Workflows holds every compiled program.WorkflowDeclaration, keyed
	// by declared name. Every Workflow here is resolved and
	// semantically validated — see engineservice's compile_workflows.go.
	Workflows map[string]Workflow

	// RootWorkflow names the Workflow a future engine step uses to
	// start a new game instance. The compiler guarantees it names an
	// entry in Workflows.
	RootWorkflow string
}

// Metadata is the compiled identity and versioning information of one
// game version, carried over unchanged from program.Metadata.
type Metadata struct {
	ID              string
	Name            string
	Description     string
	Version         string
	LanguageVersion string
}
