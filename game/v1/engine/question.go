package engine

// Question is the compiled representation of one
// program.QuestionDeclaration.
//
// Validation is nil when the source declaration had none — meaning only
// ResponseType compatibility applies to a submitted answer. When
// non-nil, the compiler guarantees it is statically bool and compiles
// against a scope containing "resources", "global", the reserved
// bindings "respondent" (user) and "answer" (ResponseType), and each of
// Parameters by name — see program.QuestionDeclaration's documented
// Validation scope.
type Question struct {
	Name         string
	Parameters   []FieldType
	ResponseType Type
	Validation   Expression
}
