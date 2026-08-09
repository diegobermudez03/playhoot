// Package program defines the source-level representation of the game
// programming language.
//
// A program.Definition is an authoring object: it describes a game in terms
// of its declared types and metadata, independently of any serialized
// format and independently of the engine that will eventually compile and
// execute it. It is not itself an executable program. Semantic compilation
// — symbol resolution, type checking, reference validation, and execution —
// belongs to a separate engine package that consumes this model.
//
// The package performs no I/O and has no infrastructure dependencies. It
// depends only on the Go standard library and must not import any engine
// implementation. The intended dependency direction is:
//
//	program <- engine
package program
