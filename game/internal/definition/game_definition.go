package definition

type numericExpression string
type boolExpression string
type flowMutation string
type globalMutation string
type flowPlayerMutation string

// expression that receives the available context and returns a single value which must be of the
// same type of the referenced field
type valueAssignmentExpression string

// expression that receives the available context (if for global event then only global and the player's state
// if for flow then it also includes flow state and player's state in that flow) and returns bool
// true if the player should be assigned or false if not, expression is ran for every player
type playersSelectionExpression string

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

// defines the operations names
const (
	forEachOp               string = "for_each"
	ifConditionOp           string = "if"
	scopeVariableCreationOp string = "scope_variable"
	assignmentOp            string = "assignment"
)

// defines interactioner types
const (
	timerInteraction     string = "timer"
	userInteraction      string = "user_input"
	waitGroupInteraction string = "wait_group"
)

// defines condition connectors
const (
	greaterConnector string = "GREATER_THAN"
	lowerConnector   string = "LOWER_THAN"
	equalConnector   string = "EQUAL_TO"
)

// defines numeric expression operations
const (
	plusOp  string = "PLUS"
	minusOp string = "MINUS"
	timesOp string = "TIMES"
	divOp   string = "DIV"
)

// defines string expression operations
const (
	joinWithOp string = "JOIN_WITH"
)

// defines list operations
const (
	// add can only add a reference into the list, code never creates a new object, it only takes the ref to a state field
	// and adds it into the list, is so that we dont have to support object creation through code
	addOp string = "ADD"
	// can only be performed inside foreach loop with the iterating item
	removeOp string = "REMOVE"
)

// defines bool expression operations
const (
	andOp string = "AND"
	orOp  string = "OR"
)

// defines wait connectors
const (
	andWaitConnector = "AND"
	orWaitConnector  = "OR"
)

type Game struct {
	// Resources is the string of the the marshaled resources
	Resources string
	// PerPlayerState is the string of the marshaled per player state
	PerPlayerStateSchema string
	// RuntimeStateSchema marshaled schema for the mutable runtime schema
	RuntimeStateSchema string
}

type Status struct {
	Name string
	Op   Operation
	// if there's an interactioner its ran before next status changer
	Interactioner Interactioner
	NextStatus    []StatusChanger
}

type StatusChanger struct {
	Condition  IfConditionOp
	GoToStatus string
}

type Interactioner interface {
	GetInteractionType() string
}

type InteractionGroup struct {
	Interactions []Interactioner
	Connectors   []string
}

func (g *InteractionGroup) GetInteractionType() string {
	return waitGroupInteraction
}

type TimerInteraction struct {
	Seconds uint
}

func (t *TimerInteraction) GetInteractionType() string {
	return timerInteraction
}

type UserInputInteraction struct {
	Params []Param
	// these assignments can only perform assignments from the params received into the state
	ThenWriteInState []AssignmentOp
}

func (t *UserInputInteraction) GetInteractionType() string {
	return userInteraction
}

type Operation interface {
	GetOperationType() string
}

type ForEachOp struct {
	ListName    string
	ItemName    string
	IterationOp Operation
}

func (f *ForEachOp) GetOperationType() string {
	return forEachOp
}

type IfConditionOp struct {
	BoolExpressions []Expression
	Connectors      []string
	IfTrue          Operation
	IfFalse         Operation
}

func (f *IfConditionOp) GetOperationType() string {
	return ifConditionOp
}

type ScopeVariableCreationOp struct {
	VariableName string
	Value        string
	Op           Operation
}

func (v *ScopeVariableCreationOp) GetOperationType() string {
	return scopeVariableCreationOp
}

type AssignmentOp struct {
	Field string
	Value string
}

func (v *AssignmentOp) GetOperationType() string {
	return assignmentOp
}

type Expression struct {
	DataType  dataType
	Value1    string
	Value2    string
	Operation string
}

// the difference between Field and Param is that Field is just a Field of the state, whereas Param is given by an user input
// therefore Param requires validation as we need to validate if user value is accepted, whereas Field is always assigned by
// Game internally so no need to validate
type Param struct {
	Name     string
	DataType dataType
	// options has to be a ref to a state list field
	OptionsField string
	Validator    ParamValidator
}

type ParamValidator struct {
	BoolExpressions []ParamValidatorExpression
	Connectors      []string
}

// difference with Expression is that here the DataType and Value1 are infered by param, param is always Value1
type ParamValidatorExpression struct {
	Value2    string
	Operation string
}
