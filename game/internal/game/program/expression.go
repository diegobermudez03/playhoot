package program

// Expression is a source-level, pure, value-producing node of the game
// language's expression language. Expressions describe how to compute a
// value; they never mutate state or trigger side effects.
//
// Expression is a closed interface. Its marker method is unexported so
// that packages outside program cannot introduce unsupported variants; the
// future compiler can safely exhaust all cases with a type switch.
type Expression interface {
	isExpression()
}

// UnitLiteralExpression represents the single value of the unit type.
type UnitLiteralExpression struct{}

func (UnitLiteralExpression) isExpression() {}

// BoolLiteralExpression represents an authored boolean literal.
type BoolLiteralExpression struct {
	Value bool
}

func (BoolLiteralExpression) isExpression() {}

// NumberLiteralExpression represents an authored numeric literal.
//
// Value is stored as the author's original textual representation (for
// example "42", "-13", or "3.1415") rather than as a runtime numeric type.
// This preserves the authored value without committing to a numeric
// representation; the future engine compiler is responsible for parsing
// and validating it.
type NumberLiteralExpression struct {
	Value string
}

func (NumberLiteralExpression) isExpression() {}

// StringLiteralExpression represents an authored string literal.
type StringLiteralExpression struct {
	Value string
}

func (StringLiteralExpression) isExpression() {}

// OptionalNoneExpression represents the absence of a value for a specific
// optional element type. ElementType makes an empty optional unambiguous
// without relying on contextual type inference.
type OptionalNoneExpression struct {
	ElementType TypeReference
}

func (OptionalNoneExpression) isExpression() {}

// OptionalSomeExpression represents the presence of a wrapped value in an
// optional.
type OptionalSomeExpression struct {
	Value Expression
}

func (OptionalSomeExpression) isExpression() {}

// ListExpression represents an authored, ordered list of elements.
//
// ElementType is an optional source annotation; a nil value means the
// future compiler should infer the element type. Empty lists will
// eventually require an explicit annotation.
type ListExpression struct {
	ElementType TypeReference
	Elements    []Expression
}

func (ListExpression) isExpression() {}

// MapExpression represents an authored key/value map.
//
// KeyType and ValueType are optional source annotations; a nil value means
// the future compiler should infer that type. Entries are stored as a
// slice, not a Go map, so that source order and duplicate keys remain
// representable until semantic validation.
type MapExpression struct {
	KeyType   TypeReference
	ValueType TypeReference
	Entries   []MapEntryExpression
}

func (MapExpression) isExpression() {}

// MapEntryExpression represents a single authored key/value entry of a
// MapExpression.
type MapEntryExpression struct {
	Key   Expression
	Value Expression
}

// EnumValueExpression references a single value of a declared enum type,
// such as Color.RED.
type EnumValueExpression struct {
	TypeName  string
	ValueName string
}

func (EnumValueExpression) isExpression() {}

// RecordExpression constructs a value of a declared record type by
// providing an initializer for each field.
type RecordExpression struct {
	TypeName string
	Fields   []FieldInitializer
}

func (RecordExpression) isExpression() {}

// FieldInitializer assigns a value to a named field within a
// RecordExpression or a union variant's fields.
type FieldInitializer struct {
	Name  string
	Value Expression
}

// UnionExpression constructs a value of a declared union type by selecting
// one variant and, if the variant has fields, providing their
// initializers. A zero-field variant is represented with an empty Fields
// slice.
type UnionExpression struct {
	TypeName    string
	VariantName string
	Fields      []FieldInitializer
}

func (UnionExpression) isExpression() {}

// NewTypeExpression constructs a value of a declared new type from an
// underlying expression. It is an explicit nominal conversion or
// construction and remains distinguishable from the wrapped expression.
type NewTypeExpression struct {
	TypeName string
	Value    Expression
}

func (NewTypeExpression) isExpression() {}

// ReferenceExpression resolves a name in the current lexical scope. The
// future compiler may provide built-in scope roots such as global,
// resources, local, signal, or viewer.
//
// Name must not encode a dotted path; field and index access are
// represented explicitly with FieldExpression and IndexExpression.
type ReferenceExpression struct {
	Name string
}

func (ReferenceExpression) isExpression() {}

// FieldExpression accesses a named field of the value produced by Target.
type FieldExpression struct {
	Target Expression
	Field  string
}

func (FieldExpression) isExpression() {}

// IndexExpression accesses the value produced by Target at the key or
// position produced by Index.
type IndexExpression struct {
	Target Expression
	Index  Expression
}

func (IndexExpression) isExpression() {}

// UnaryOperator identifies a supported unary operation.
type UnaryOperator string

const (
	// UnaryOperatorNot performs boolean negation.
	UnaryOperatorNot UnaryOperator = "not"

	// UnaryOperatorNegate performs numeric negation.
	UnaryOperatorNegate UnaryOperator = "negate"
)

// IsValid reports whether o is one of the unary operators supported by
// this package.
func (o UnaryOperator) IsValid() bool {
	switch o {
	case UnaryOperatorNot, UnaryOperatorNegate:
		return true
	default:
		return false
	}
}

// UnaryExpression applies a unary operator to a single operand. The future
// compiler is responsible for validating that the operand's type is
// compatible with the operator.
type UnaryExpression struct {
	Operator UnaryOperator
	Operand  Expression
}

func (UnaryExpression) isExpression() {}

// BinaryOperator identifies a supported binary operation.
type BinaryOperator string

const (
	// BinaryOperatorAdd adds two compatible numeric values.
	BinaryOperatorAdd BinaryOperator = "add"
	// BinaryOperatorSubtract subtracts two compatible numeric values.
	BinaryOperatorSubtract BinaryOperator = "subtract"
	// BinaryOperatorMultiply multiplies two compatible numeric values.
	BinaryOperatorMultiply BinaryOperator = "multiply"
	// BinaryOperatorDivide divides two compatible numeric values.
	BinaryOperatorDivide BinaryOperator = "divide"
	// BinaryOperatorModulo computes the remainder of two compatible
	// numeric values.
	BinaryOperatorModulo BinaryOperator = "modulo"

	// BinaryOperatorEqual tests two compatible values for equality.
	BinaryOperatorEqual BinaryOperator = "equal"
	// BinaryOperatorNotEqual tests two compatible values for inequality.
	BinaryOperatorNotEqual BinaryOperator = "not_equal"
	// BinaryOperatorLess tests whether the left value orders before the
	// right value.
	BinaryOperatorLess BinaryOperator = "less"
	// BinaryOperatorLessOrEqual tests whether the left value orders before
	// or equal to the right value.
	BinaryOperatorLessOrEqual BinaryOperator = "less_or_equal"
	// BinaryOperatorGreater tests whether the left value orders after the
	// right value.
	BinaryOperatorGreater BinaryOperator = "greater"
	// BinaryOperatorGreaterOrEqual tests whether the left value orders
	// after or equal to the right value.
	BinaryOperatorGreaterOrEqual BinaryOperator = "greater_or_equal"

	// BinaryOperatorAnd is the short-circuiting logical AND.
	BinaryOperatorAnd BinaryOperator = "and"
	// BinaryOperatorOr is the short-circuiting logical OR.
	BinaryOperatorOr BinaryOperator = "or"

	// BinaryOperatorIn tests membership of the left value in the
	// collection produced by the right value.
	BinaryOperatorIn BinaryOperator = "in"
	// BinaryOperatorNotIn tests non-membership of the left value in the
	// collection produced by the right value.
	BinaryOperatorNotIn BinaryOperator = "not_in"
)

// IsValid reports whether o is one of the binary operators supported by
// this package.
func (o BinaryOperator) IsValid() bool {
	switch o {
	case BinaryOperatorAdd, BinaryOperatorSubtract, BinaryOperatorMultiply, BinaryOperatorDivide, BinaryOperatorModulo,
		BinaryOperatorEqual, BinaryOperatorNotEqual, BinaryOperatorLess, BinaryOperatorLessOrEqual, BinaryOperatorGreater, BinaryOperatorGreaterOrEqual,
		BinaryOperatorAnd, BinaryOperatorOr,
		BinaryOperatorIn, BinaryOperatorNotIn:
		return true
	default:
		return false
	}
}

// BinaryExpression applies a binary operator to a left and right operand.
// The source package only represents the operation; type checking and
// evaluation belong to the future engine.
type BinaryExpression struct {
	Operator BinaryOperator
	Left     Expression
	Right    Expression
}

func (BinaryExpression) isExpression() {}

// ConditionalExpression is a pure expression equivalent to
// "if Condition then Then else Else". It does not perform operations or
// state mutations; both branches must eventually produce compatible
// types, but that is not validated by this package.
type ConditionalExpression struct {
	Condition Expression
	Then      Expression
	Else      Expression
}

func (ConditionalExpression) isExpression() {}

// CallExpression invokes a pure, value-producing function by name using
// named arguments.
//
// CallExpression represents pure calls only; it must not be used for
// mutating state, spawning workflows, asking users, scheduling timers, or
// emitting UI effects. Those are modeled as operations in a later step.
// The future compiler validates function existence, missing or unknown
// arguments, duplicate arguments, argument types, and whether the target
// is a built-in or user-declared pure function.
type CallExpression struct {
	Function  string
	Arguments []CallArgument
}

func (CallExpression) isExpression() {}

// CallArgument binds a named argument of a CallExpression to a value
// expression.
type CallArgument struct {
	Name  string
	Value Expression
}
