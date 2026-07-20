package definition

type dataType string

// defines data types names
const (
	stringType  dataType = "string"
	numericType dataType = "numeric"
	boolType    dataType = "bool"
	refType     dataType = "ref"
	objType     dataType = "obj"
	listType    dataType = "list"
)

type operation string

// defines the operations names
const (
	forEachOp               operation = "for_each"
	ifConditionOp           operation = "if"
	scopeVariableCreationOp operation = "scope_variable"
	assignmentOp            operation = "assignment"
)

type interactionType string

// defines interactioner types
const (
	timerInteraction     interactionType = "timer"
	userInteraction      interactionType = "user_input"
	waitGroupInteraction interactionType = "wait_group"
)

type conditionOperation string

// defines condition connectors
const (
	greaterOp conditionOperation = "GREATER_THAN"
	lowerOp   conditionOperation = "LOWER_THAN"
	equalOp   conditionOperation = "EQUAL_TO"
)

type numericOperation string

// defines numeric expression operations
const (
	plusOp  numericOperation = "PLUS"
	minusOp numericOperation = "MINUS"
	timesOp numericOperation = "TIMES"
	divOp   numericOperation = "DIV"
)

type stringOperation string

// defines string expression operations
const (
	joinWithOp stringOperation = "JOIN_WITH"
)

type listOperation string

// defines list operations
const (
	// add can only add a reference into the list, code never creates a new object, it only takes the ref to a state field
	// and adds it into the list, is so that we dont have to support object creation through code
	addOp listOperation = "ADD"
	// can only be performed inside foreach loop with the iterating item
	removeOp listOperation = "REMOVE"
)

type boolConnector string

// defines bool connectors
const (
	andConnector boolConnector = "AND"
	orConnector  boolConnector = "OR"
)

// defines wait connectors
const (
	andWaitConnector = "AND"
	orWaitConnector  = "OR"
)

type Game struct {
	// Resources is the string of the the marshaled resources
	Resources ObjectType
	// PerPlayerState is the string of the marshaled per player state
	PlayersState ListType
	// RuntimeStateSchema marshaled schema for the mutable runtime schema
	RuntimeStateSchema ObjectType
	Statuses           []Status
	InitialStatus      string
}

type Status struct {
	Name       string
	Operations []Operation
	// if there's an interactioner its ran before next status changer
	Interactioner Interactioner
	NextStatus    []StatusChanger
}

type StatusChanger struct {
	Condition  IfConditionOp
	GoToStatus string
}

type Interactioner interface {
	GetInteractionType() interactionType
}

type InteractionGroup struct {
	Interactions []Interactioner
	Connectors   []boolConnector
}

func (g InteractionGroup) GetInteractionType() interactionType {
	return waitGroupInteraction
}

type TimerInteraction struct {
	Seconds uint
}

func (t TimerInteraction) GetInteractionType() interactionType {
	return timerInteraction
}

type UserInputInteraction struct {
	Params []Param
	// these assignments can only perform assignments from the params received into the state
	ThenWriteInState []AssignmentOp
	// state field which contains ref to the users
	UsersToInteractWithField RefType
}

func (t UserInputInteraction) GetInteractionType() interactionType {
	return userInteraction
}

type Operation interface {
	GetOperationType() operation
}

type ForEachOp struct {
	List         RefType
	ItemName     string
	IterationOps []Operation
}

func (f ForEachOp) GetOperationType() operation {
	return forEachOp
}

type IfConditionOp struct {
	BoolExpressions []BoolExpression
	Connectors      []boolConnector
	IfTrue          []Operation
	IfFalse         []Operation
}

func (f IfConditionOp) GetOperationType() operation {
	return ifConditionOp
}

type ScopeVariableCreationOp struct {
	VariableName string
	Value        ValueType
	Ops          []Operation
}

func (v ScopeVariableCreationOp) GetOperationType() operation {
	return scopeVariableCreationOp
}

type AssignmentOp struct {
	Field RefType
	Value Expression
}

func (v AssignmentOp) GetOperationType() operation {
	return assignmentOp
}

type Expression interface {
	GetExpressionDataType() dataType
}

type BoolExpression struct {
	Value1    RefType
	Value2    RefType
	Operation conditionOperation
}

func (e BoolExpression) GetExpressionDataType() dataType {
	return boolType
}

type NumericExpression struct {
	Value1    RefType
	Value2    RefType
	operation numericOperation
}

func (e NumericExpression) GetExpressionDataType() dataType {
	return numericType
}

type StringExpression struct {
	Value1    RefType
	Value2    RefType
	operation stringOperation
}

func (e StringExpression) GetExpressionDataType() dataType {
	return stringType
}

type ListExpression struct {
	ListRef   RefType
	Operation listOperation
	Value     *RefType
}

func (e ListExpression) GetExpressionDataType() dataType {
	return listType
}

type Param interface {
	GetParamDataType() dataType
}

type StringParam struct {
	Name       string
	FixOptions []string
	RefOptions RefType
}

func (p StringParam) GetParamDataType() dataType {
	return stringType
}

type NumericParam struct {
	Name                            string
	Options                         []float64
	RefOptions                      RefType
	ValidationExpressions           []NumericParamValidator
	ValidationExpressionsConnectors []boolConnector
}

func (p *NumericParam) GetParamDataType() dataType {
	return numericType
}

type BoolParam struct {
	Name string
}

func (p BoolParam) GetParamDataType() dataType {
	return boolType
}

type NumericParamValidator struct {
	Value2    RefType
	Operation conditionOperation
}

type ValueType interface {
	GetDataType() dataType
}

type StringType struct {
	Value string
}

func (t StringType) GetDataType() dataType {
	return stringType
}

type NumericType struct {
	Value float64
}

func (t NumericType) GetDataType() dataType {
	return numericType
}

type BoolType struct {
	Value bool
}

func (t BoolType) GetDataType() dataType {
	return stringType
}

type RefType struct {
	// stores the compositions like 'State' 'Obj1' 'field1'
	VariableComposition []string
}

func (t RefType) GetDataType() dataType {
	return refType
}

type ObjectType struct {
	Fields map[string]ValueType
}

func (t ObjectType) GetDataType() dataType {
	return objType
}

type ListType struct {
	// for primitive types this is redundant, but for objects we need a clear schema
	Type   ValueType
	Values []ValueType
}

func (t ListType) GetDataType() dataType {
	return listType
}
