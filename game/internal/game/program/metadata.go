package program

// Metadata describes the identity and versioning of a game definition.
type Metadata struct {
	// ID is the stable identifier of the game definition.
	ID string

	// Name is the human-readable name of the game definition.
	Name string

	// Description is a human-readable summary of the game definition.
	Description string

	// Version identifies the authored game version. This package does not
	// validate its syntax.
	Version string

	// LanguageVersion identifies the version of the game language expected
	// by the definition. This package does not validate its syntax.
	LanguageVersion string
}
