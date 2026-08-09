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
}
