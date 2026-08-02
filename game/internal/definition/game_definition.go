package definition

type dataType string

// defines data types names
const (
	stringType       dataType = "string"
	numericType      dataType = "numeric"
	boolType         dataType = "bool"
	listItemRef      dataType = "list_item_ref"
	objType          dataType = "obj"
	listType         dataType = "list"
	playersRefType   dataType = "player_ref"
	numericConstType dataType = "numeric_const"
	stringConstType  dataType = "string_const"
	boolConstType    dataType = "bool_const"
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

type variableComposition []string

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

// --------------------------------------------------------------------
type Game struct {
	Resources     ObjectType
	PlayersState  ObjectType
	State         ObjectType
	Statuses      []Status
	InitialStatus string
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

// --------------------------------------------------------------------

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
	UsersToInteractWithField PlayesrRefType
}

func (t UserInputInteraction) GetInteractionType() interactionType {
	return userInteraction
}

// --------------------------------------------------------------------

type Operation interface {
	GetOperationType() operation
}

type ForEachOp struct {
	List         variableComposition
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
	Field variableComposition
	Value Expression
}

func (v AssignmentOp) GetOperationType() operation {
	return assignmentOp
}

// --------------------------------------------------------------------

type Expression interface {
	GetExpressionDataType() dataType
}

type BoolExpression struct {
	// commented out because I dont know yet how to define these as they should be either consts or variable compositions of the type
	// Value1
	// Value2
	Operation conditionOperation
}

func (e BoolExpression) GetExpressionDataType() dataType {
	return boolType
}

type NumericExpression struct {
	// commented out because I dont know yet how to define these as they should be either consts or variable compositions of the type
	// Value1
	// Value2
	operation numericOperation
}

func (e NumericExpression) GetExpressionDataType() dataType {
	return numericType
}

type StringExpression struct {
	// commented out because I dont know yet how to define these as they should be either consts or variable compositions of the type
	// Value1
	// Value2
	operation stringOperation
}

func (e StringExpression) GetExpressionDataType() dataType {
	return stringType
}

type ListExpression struct {
	ListRef variableComposition
	// if its a remove operation there's no need to pass a variable, that operation
	// can only be called while iterating the list and when called its inferred that its the iterating item at the moment
	Operation listOperation
	// Add operations can only add a value from an existing variable
	Variable variableComposition
}

func (e ListExpression) GetExpressionDataType() dataType {
	return listType
}

// --------------------------------------------------------------------

type Param interface {
	GetParamDataType() dataType
}

type StringParam struct {
	Name string
	// if the param has const options
	FixOptions []string
	// if the param has dynamic options, then it must reference a list of string type
	RefOptions variableComposition
}

func (p StringParam) GetParamDataType() dataType {
	return stringType
}

type NumericParam struct {
	Name string
	// if the param has const options
	Options []float64
	// if the param has dynamic options, then it must reference a list of numeric type
	RefOptions                      variableComposition
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

// NumericParamValidator contains the value against to which to validate the param and the validation operation
type NumericParamValidator struct {
	// commented out as I dont know yet how to represent this that could be either a variable composition or a const
	// Value2
	Operation conditionOperation
}

// --------------------------------------------------------------------

type ValueType interface {
	GetDataType() dataType
}

type StringType struct{}

func (t StringType) GetDataType() dataType {
	return stringType
}

type NumericType struct{}

func (t NumericType) GetDataType() dataType {
	return numericType
}

type BoolType struct{}

func (t BoolType) GetDataType() dataType {
	return stringType
}

// references an item from the list, list must be of non primitive (numeric, bool, string) type
// the reference is direct so if a change is performed on this will affect the referenced variable
// Just for Dev context, recall this is just the definition, the actual engine will have to store the index
type ListItemRefType struct {
	// stores the compositions like 'State' 'Obj1' 'field1'
	ListVariableComposition variableComposition
}

func (t ListItemRefType) GetDataType() dataType {
	return listItemRef
}

type ObjectType struct {
	Fields map[string]ValueType
}

func (t ObjectType) GetDataType() dataType {
	return objType
}

type ListType struct {
	// for primitive types this is redundant, but for objects we need a clear schema
	Type ValueType
}

func (t ListType) GetDataType() dataType {
	return listType
}

type PlayesrRefType struct{}

func (t PlayesrRefType) GetDataType() dataType {
	return playersRefType
}

type NumericConstType struct {
	Value float64
}

func (t NumericConstType) GetDataType() dataType {
	return playersRefType
}

type StringConstType struct {
	Value string
}

func (t StringConstType) GetDataType() dataType {
	return stringConstType
}

type BoolConstType struct {
	Value bool
}

func (t BoolConstType) GetDataType() dataType {
	return boolConstType
}
