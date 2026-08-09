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
// Program is intentionally incomplete and will continue to grow: this
// version does not yet define any compiled content — compiled
// workflows, symbol tables, executable instructions — those fields are
// added as compilation semantics are implemented.
type Program struct{}
