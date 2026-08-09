package engine

// StateField is the compiled representation of one field of
// program.Definition.GlobalState, mirroring program.StateFieldDeclaration
// once compiled: Type is resolved, and Initializer is compiled and known
// to statically match Type.
//
// A StateField is evaluated once per new game instance — see
// engineservice.NewSnapshot — not once per compiled Program, since each
// instance owns an independent copy of global state.
type StateField struct {
	Name        string
	Type        Type
	Initializer Expression
}
