package engine

// UnaryOperator identifies a supported unary operation, mirroring
// program.UnaryOperator once compiled. Unlike program's version, a
// compiled UnaryExpression's Operator is always one of these constants —
// an unrecognized operator is rejected during compilation, not preserved.
type UnaryOperator string

const (
	UnaryOperatorNot    UnaryOperator = "not"
	UnaryOperatorNegate UnaryOperator = "negate"
)

// BinaryOperator identifies a supported binary operation, mirroring
// program.BinaryOperator once compiled. Unlike program's version, a
// compiled BinaryExpression's Operator is always one of these constants.
type BinaryOperator string

const (
	BinaryOperatorAdd      BinaryOperator = "add"
	BinaryOperatorSubtract BinaryOperator = "subtract"
	BinaryOperatorMultiply BinaryOperator = "multiply"
	BinaryOperatorDivide   BinaryOperator = "divide"
	BinaryOperatorModulo   BinaryOperator = "modulo"

	BinaryOperatorEqual          BinaryOperator = "equal"
	BinaryOperatorNotEqual       BinaryOperator = "not_equal"
	BinaryOperatorLess           BinaryOperator = "less"
	BinaryOperatorLessOrEqual    BinaryOperator = "less_or_equal"
	BinaryOperatorGreater        BinaryOperator = "greater"
	BinaryOperatorGreaterOrEqual BinaryOperator = "greater_or_equal"

	BinaryOperatorAnd BinaryOperator = "and"
	BinaryOperatorOr  BinaryOperator = "or"

	BinaryOperatorIn    BinaryOperator = "in"
	BinaryOperatorNotIn BinaryOperator = "not_in"
)

// Expression is the engine's compiled representation of one pure,
// value-producing computation, mirroring program.Expression once
// compiled: every name is resolved (a type constructor's TypeName is
// known to exist and be the right kind, a ReferenceExpression's Name is
// known to be bound, a CallExpression's Function is known to exist and
// its arguments known to match), every TypeReference is a resolved Type,
// and a NumberLiteralExpression's textual Value is parsed into a Go
// float64. Compiling an Expression also determines its static Type; see
// engineservice.Compile.
//
// Expression is a closed interface, mirroring program's own
// closed-interface pattern: its marker method is unexported so packages
// outside engine cannot introduce unsupported variants, and a type
// switch inside this package — or inside engineservice, which compiles
// and evaluates Expression — can safely be treated as exhaustive.
type Expression interface {
	isExpression()
}

// UnitLiteralExpression is the single value of the unit type.
type UnitLiteralExpression struct{}

func (UnitLiteralExpression) isExpression() {}

// BoolLiteralExpression is a boolean literal.
type BoolLiteralExpression struct {
	Value bool
}

func (BoolLiteralExpression) isExpression() {}

// NumberLiteralExpression is a numeric literal, parsed at compile time
// from program.NumberLiteralExpression's textual representation.
type NumberLiteralExpression struct {
	Value float64
}

func (NumberLiteralExpression) isExpression() {}

// StringLiteralExpression is a text literal.
type StringLiteralExpression struct {
	Value string
}

func (StringLiteralExpression) isExpression() {}

// OptionalNoneExpression evaluates to the absent value of the optional
// type wrapping ElementType.
type OptionalNoneExpression struct {
	ElementType Type
}

func (OptionalNoneExpression) isExpression() {}

// OptionalSomeExpression evaluates to the present value of the optional
// type wrapping ElementType, wrapping the result of Value. ElementType is
// Value's compiled static type, recorded here because the resulting
// OptionalValue needs it and program's source form has no such field.
type OptionalSomeExpression struct {
	ElementType Type
	Value       Expression
}

func (OptionalSomeExpression) isExpression() {}

// ListExpression constructs an ordered list. Unlike
// program.ListExpression, ElementType is never nil: the compiler either
// resolves an explicit annotation or infers it from Elements, and
// rejects an empty list with neither.
type ListExpression struct {
	ElementType Type
	Elements    []Expression
}

func (ListExpression) isExpression() {}

// MapExpression constructs a key/value map. As with ListExpression,
// KeyType and ValueType are never nil.
type MapExpression struct {
	KeyType   Type
	ValueType Type
	Entries   []MapEntryExpression
}

func (MapExpression) isExpression() {}

// MapEntryExpression is a single compiled key/value entry of a
// MapExpression.
type MapEntryExpression struct {
	Key   Expression
	Value Expression
}

// EnumValueExpression references one symbolic value of the named,
// resolved enum type TypeName.
type EnumValueExpression struct {
	TypeName  string
	ValueName string
}

func (EnumValueExpression) isExpression() {}

// RecordExpression constructs a value of the named, resolved record type
// TypeName, providing exactly its declared Fields.
type RecordExpression struct {
	TypeName string
	Fields   []FieldInitializer
}

func (RecordExpression) isExpression() {}

// FieldInitializer assigns a compiled Value expression to the named
// field Name within a RecordExpression or a union variant's fields.
type FieldInitializer struct {
	Name  string
	Value Expression
}

// UnionExpression constructs a value of the named, resolved union type
// TypeName, selecting VariantName and providing that variant's Fields.
type UnionExpression struct {
	TypeName    string
	VariantName string
	Fields      []FieldInitializer
}

func (UnionExpression) isExpression() {}

// NewTypeExpression constructs a value of the named, resolved new type
// TypeName, wrapping Value.
type NewTypeExpression struct {
	TypeName string
	Value    Expression
}

func (NewTypeExpression) isExpression() {}

// ReferenceExpression reads the immutable lexical binding Name from the
// current evaluation Scope. The compiler guarantees Name was declared in
// scope at compile time; Evaluate still reports a missing binding as an
// error rather than panicking, since a caller-supplied Scope is outside
// the compiler's control.
type ReferenceExpression struct {
	Name string
}

func (ReferenceExpression) isExpression() {}

// FieldExpression accesses the named Field of the record value produced
// by Target. Only records support field access in this version — a
// union's fields vary by variant, so accessing one requires
// MatchExpression instead.
type FieldExpression struct {
	Target Expression
	Field  string
}

func (FieldExpression) isExpression() {}

// IndexExpression accesses the value produced by Target — a list or a
// map — at the position or key produced by Index.
type IndexExpression struct {
	Target Expression
	Index  Expression
}

func (IndexExpression) isExpression() {}

// UnaryExpression applies Operator to Operand.
type UnaryExpression struct {
	Operator UnaryOperator
	Operand  Expression
}

func (UnaryExpression) isExpression() {}

// BinaryExpression applies Operator to Left and Right.
//
// Operator BinaryOperatorAnd and BinaryOperatorOr short-circuit at
// evaluation: Right is only evaluated when Left does not already
// determine the result.
type BinaryExpression struct {
	Operator BinaryOperator
	Left     Expression
	Right    Expression
}

func (BinaryExpression) isExpression() {}

// ConditionalExpression evaluates Condition and evaluates exactly one of
// Then or Else — the other is never evaluated.
type ConditionalExpression struct {
	Condition Expression
	Then      Expression
	Else      Expression
}

func (ConditionalExpression) isExpression() {}

// CallExpression invokes the pure function or built-in named Function
// with Arguments. The compiler guarantees Function names either a
// compiled Program.Functions entry or an entry in engineservice's
// built-in catalog, never both, and that Arguments exactly matches the
// target's declared parameters.
type CallExpression struct {
	Function  string
	Arguments []CallArgument
}

func (CallExpression) isExpression() {}

// CallArgument binds the named argument Name of a CallExpression to the
// compiled Value expression Value.
type CallArgument struct {
	Name  string
	Value Expression
}

// MatchExpression evaluates Value once and evaluates the Result of the
// first case in Cases whose Pattern matches — see MatchPattern. Evaluate
// reports an error, rather than panicking, if no case matches; the
// compiler does not yet require Cases to be exhaustive.
type MatchExpression struct {
	Value Expression
	Cases []MatchExpressionCase
}

func (MatchExpression) isExpression() {}

// MatchExpressionCase pairs one MatchPattern with the Result evaluated
// when that pattern is selected. Pattern's lexical bindings, if any, are
// in scope only within Result.
type MatchExpressionCase struct {
	Pattern MatchPattern
	Result  Expression
}

// ListMapExpression transforms every element of Collection into one
// Result value, producing a new list. ResultElementType is Result's
// compiled static type, recorded here because program's source form has
// no such field but the resulting ListValue needs it.
type ListMapExpression struct {
	Collection        Expression
	ItemName          string
	IndexName         string
	Result            Expression
	ResultElementType Type
}

func (ListMapExpression) isExpression() {}

// ListFilterExpression retains the elements of Collection for which
// Predicate evaluates to true, preserving order and element type.
type ListFilterExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListFilterExpression) isExpression() {}

// ListFlatMapExpression maps each element of Collection to a list via
// Result and concatenates the results in order. ResultElementType is the
// element type of each per-element list Result produces (so the flat
// result list's element type), recorded for the same reason as
// ListMapExpression.ResultElementType.
type ListFlatMapExpression struct {
	Collection        Expression
	ItemName          string
	IndexName         string
	Result            Expression
	ResultElementType Type
}

func (ListFlatMapExpression) isExpression() {}

// ListAnyExpression evaluates to true if Predicate is true for at least
// one element of Collection. Evaluation stops at the first element for
// which Predicate is true — a later element's Predicate is never
// evaluated once the result is already determined.
type ListAnyExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListAnyExpression) isExpression() {}

// ListAllExpression evaluates to true if Predicate is true for every
// element of Collection. Evaluation stops at the first element for which
// Predicate is false.
type ListAllExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListAllExpression) isExpression() {}

// ListCountExpression evaluates to the number of elements of Collection
// for which Predicate is true. Unlike ListAnyExpression and
// ListAllExpression, this never short-circuits: Predicate is evaluated
// for every element.
type ListCountExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListCountExpression) isExpression() {}

// ListFirstExpression evaluates, in order, to the first element of
// Collection for which Predicate is true, wrapped as a present optional
// value, stopping at that first match; it evaluates to an absent
// optional if none matches.
type ListFirstExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListFirstExpression) isExpression() {}
