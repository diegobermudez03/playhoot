package program

// UIEventType identifies a client UI event a view may react to.
type UIEventType string

const (
	// UIEventTypeClick fires on a single click or tap.
	UIEventTypeClick UIEventType = "click"
	// UIEventTypeDoubleClick fires on a double click or double tap.
	UIEventTypeDoubleClick UIEventType = "double_click"
	// UIEventTypePointerEnter fires when a pointer starts hovering an
	// element.
	UIEventTypePointerEnter UIEventType = "pointer_enter"
	// UIEventTypePointerLeave fires when a pointer stops hovering an
	// element.
	UIEventTypePointerLeave UIEventType = "pointer_leave"
)

// IsValid reports whether e is one of the UI event types supported by this
// package.
func (e UIEventType) IsValid() bool {
	switch e {
	case UIEventTypeClick, UIEventTypeDoubleClick, UIEventTypePointerEnter, UIEventTypePointerLeave:
		return true
	default:
		return false
	}
}

// UIEventHandler runs Actions, in declaration order, whenever Event fires
// on the element it is attached to.
//
// These initial event types expose no event-payload fields to the
// authored language, so there is no implicit "event" lexical binding in
// this step; future input, drag, keyboard, or pointer events may introduce
// typed event payloads separately.
type UIEventHandler struct {
	Event   UIEventType
	Actions []UIAction
}

// UIAction is a single client-side action executed by a UIEventHandler.
//
// UIAction is a closed interface, entirely separate from the server-side
// Operation vocabulary: a UI action tree never contains Block, Operation,
// or WorkflowControl values, and no UIAction mutates authoritative game
// state directly. Its marker method is unexported so that packages
// outside program cannot introduce unsupported variants; the future
// compiler can safely exhaust all cases with a type switch.
type UIAction interface {
	isUIAction()
}

// SetLocalStateAction assigns the result of Value to Target within the
// mounted view's own client-local state.
//
// Target reuses the existing AssignmentTarget model and must root at
// "local"; this action must never modify model, global, resources, or
// workflow-local state. Performing this action causes the mounted view to
// rerender.
type SetLocalStateAction struct {
	Target AssignmentTarget
	Value  Expression
}

func (SetLocalStateAction) isUIAction() {}

// AnswerQuestionAction submits the result of Value as the answer to the
// question instance currently active for the mounted view.
//
// This action is only valid when the containing view is mounted through a
// pending question presentation (see QuestionPresentationDeclaration): the
// mounted question context, not this action, identifies the concrete
// question instance being answered, along with its recipient, response
// type, and authoritative validator — which is why this action carries no
// question-slot name. A ViewDeclaration containing AnswerQuestionAction
// remains representable before any usage analysis; the future compiler
// must reject using such a view in a non-question presentation unless the
// action is proven unreachable under some future supported analysis. This
// package performs no such validation.
//
// The action evaluates Value and submits it; it does not mutate
// authoritative state directly and does not guarantee the answer is
// accepted — the future authoritative engine independently validates
// recipient identity, response type, and the question's Validation
// expression, and a rejected answer never produces a workflow signal.
type AnswerQuestionAction struct {
	Value Expression
}

func (AnswerQuestionAction) isUIAction() {}

// EmitUserIntentAction submits one instance of the named
// UserIntentDeclaration, with Arguments evaluated in declaration order.
//
// Intent is a static declaration name. The connected user becomes the
// implicit authoritative actor; this action sends the typed intent to the
// server but does not mutate authoritative state directly; the server may
// reject the intent based on workflow state, guards, authorization, or
// payload validation.
type EmitUserIntentAction struct {
	Intent    string
	Arguments []CallArgument
}

func (EmitUserIntentAction) isUIAction() {}
