package program

// BuiltinType identifies a platform-provided type available to every game
// definition without a corresponding user-declared type.
type BuiltinType string

const (
	// BuiltinTypeUnit represents the absence of a meaningful value.
	BuiltinTypeUnit BuiltinType = "unit"

	// BuiltinTypeBool represents a boolean value.
	BuiltinTypeBool BuiltinType = "bool"

	// BuiltinTypeNumber represents a numeric value.
	BuiltinTypeNumber BuiltinType = "number"

	// BuiltinTypeString represents a text value.
	BuiltinTypeString BuiltinType = "string"

	// BuiltinTypeUser represents a real user connected to a room. It is a
	// platform-provided type; this package only declares that it exists.
	// Its capabilities and runtime meaning are defined by the engine that
	// compiles and executes the definition.
	BuiltinTypeUser BuiltinType = "user"
)

// IsValid reports whether t is one of the built-in types supported by this
// package.
func (t BuiltinType) IsValid() bool {
	switch t {
	case BuiltinTypeUnit, BuiltinTypeBool, BuiltinTypeNumber, BuiltinTypeString, BuiltinTypeUser:
		return true
	default:
		return false
	}
}
