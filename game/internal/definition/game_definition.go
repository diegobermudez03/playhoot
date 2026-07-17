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
	enumType    dataType = "enum"
)

type Game struct {
	State          GlobalState
	PerPlayerState PlayerState
	Flows          []Flow
}

type GlobalState struct {
	Payload map[string]Field
	Events  []GlobalEvent
}

type PlayerState struct {
	InitialParams map[string]Param
	State         map[string]Field
	Events        []UIEvent
}

type UIEvent struct {
}

type Field struct {
	Name         string
	TypeName     string
	InitialValue string
	Options      []string
}

// the difference between Field and Param is that Field is just a Field of the state, whereas Param is given by an user input
// therefore Param requires validation as we need to validate if user value is accepted, whereas Field is always assigned by
// Game internally so no need to validate
type Param struct {
	Field
	// expression which validates that the input is accepted
	ValidationExpresion string
}

type GlobalEvent struct {
	Conditions []boolExpression
	// ObserveFields defines based on which fields change is the event going to be emitted
	// if empty the event will be emitted on every global state change
	ObserveFields []string
	Do            []FlowTrigger
	PauseFlows    []string
	KillFlows     []string
	UnpauseFlows  []string
}

type GlobalPlayerEvent struct {
	AllowedIfFlowExists []string
	// expression based on global player state
	PlayerAllowedExpression boolExpression
	Params                  []Param
	Do                      []FlowTrigger
	PauseFlows              []string
	KillFlows               []string
	UnpauseFlows            []string
}

type FlowTrigger struct {
	FlowName         string
	PlayersSelection playersSelectionExpression
	// in this case the assignment expression will also receive the specific player
	// as input
	PerPlayerPayloadAssigner []FieldValueAssigner
	InitialStatus            string
	StatusStateAssigner      []FieldValueAssigner
}

type FieldValueAssigner struct {
	FieldName            string
	AssignmentExpression valueAssignmentExpression
}

type Flow struct {
	Name        string
	PlayerState PlayerState
}

type Status struct {
	Name                string
	StatusState         map[string]Field
	ThenAskPlayer       *AskPlayer
	WaitForSeconds      uint
	WaitForAllUsersResp bool
	ThenDo              []StatusChange
}

type AskPlayer struct {
	Params                []Param
	PlayersToAskSelection playersSelectionExpression
}

type StatusChange struct {
	Conditions                 []boolExpression
	GoToStatus                 string
	StatusStateAssigner        []FieldValueAssigner
	PerPlayerFlowStateChange   []FieldValueAssigner
	UpdateGlobalState          []FieldValueAssigner
	UpdatePerPlayerGlobalState []FieldValueAssigner
}
