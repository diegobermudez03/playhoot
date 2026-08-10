package engine

// UILayout is the compiled representation of one program.UILayout
// variant, mirroring program's closed-interface pattern.
type UILayout interface {
	isUILayout()
}

// StackLayout places children in the same logical region, allowing
// them to visually overlap.
type StackLayout struct{}

func (StackLayout) isUILayout() {}

// AbsoluteLayout positions each child independently through that
// child's own properties.
type AbsoluteLayout struct{}

func (AbsoluteLayout) isUILayout() {}

// LinearLayoutDirection identifies the axis a LinearLayout arranges
// children along.
type LinearLayoutDirection string

const (
	LinearLayoutDirectionRow    LinearLayoutDirection = "row"
	LinearLayoutDirectionColumn LinearLayoutDirection = "column"
)

// LinearLayout arranges children one after another along Direction.
// Gap is nil when no explicit spacing was authored.
type LinearLayout struct {
	Direction LinearLayoutDirection
	Gap       Expression
}

func (LinearLayout) isUILayout() {}

// GridLayout arranges children into a grid of Columns columns. RowGap
// and ColumnGap are nil when no explicit spacing was authored.
type GridLayout struct {
	Columns   Expression
	RowGap    Expression
	ColumnGap Expression
}

func (GridLayout) isUILayout() {}

// UIElement is the compiled representation of one program.UIElement
// variant — a node of a view's declarative UI element tree, mirroring
// program's closed-interface pattern. The engine only compiles and
// validates this tree; it never renders it — see program.UIElement's
// documented purity with respect to model, local state, and repeat
// lexical bindings.
type UIElement interface {
	isUIElement()
}

// EmptyElement renders nothing.
type EmptyElement struct{}

func (EmptyElement) isUIElement() {}

// ContainerElement groups Children under one explicit Layout.
type ContainerElement struct {
	Configuration UIElementConfiguration
	Layout        UILayout
	Children      []UIElement
}

func (ContainerElement) isUIElement() {}

// TextElement renders Value as text. The compiler guarantees Value is
// statically string.
type TextElement struct {
	Configuration UIElementConfiguration
	Value         Expression
}

func (TextElement) isUIElement() {}

// ImageElement renders an image identified by Source. AlternativeText
// is nil when none was authored.
type ImageElement struct {
	Configuration   UIElementConfiguration
	Source          Expression
	AlternativeText Expression
}

func (ImageElement) isUIElement() {}

// ButtonElement is an interactive visual container whose visible
// content is its Children.
type ButtonElement struct {
	Configuration UIElementConfiguration
	Children      []UIElement
}

func (ButtonElement) isUIElement() {}

// RepeatElement renders one instance of Body for every value produced
// by Collection, with ItemName (and, if non-empty, IndexName) bound
// within Body. Key is nil when none was authored. The compiler
// guarantees Collection is statically a list.
type RepeatElement struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Key        Expression
	Body       UIElement
}

func (RepeatElement) isUIElement() {}

// ConditionalElement renders exactly one of Then or Else depending on
// Condition. The compiler guarantees Condition is statically bool.
type ConditionalElement struct {
	Condition Expression
	Then      UIElement
	Else      UIElement
}

func (ConditionalElement) isUIElement() {}

// UIElementConfiguration groups the statically named properties and
// client event handlers attached to a UI element.
type UIElementConfiguration struct {
	Properties []UIProperty
	Events     []UIEventHandler
}

// UIProperty assigns Value to the statically named renderer capability
// Name.
type UIProperty struct {
	Name  string
	Value Expression
}

// UIEventType identifies a client UI event a view may react to.
type UIEventType string

const (
	UIEventTypeClick        UIEventType = "click"
	UIEventTypeDoubleClick  UIEventType = "double_click"
	UIEventTypePointerEnter UIEventType = "pointer_enter"
	UIEventTypePointerLeave UIEventType = "pointer_leave"
)

// UIEventHandler runs Actions, in declaration order, whenever Event
// fires on the element it is attached to.
type UIEventHandler struct {
	Event   UIEventType
	Actions []UIAction
}

// UIAction is the compiled representation of one program.UIAction
// variant: a single client-side action executed by a UIEventHandler,
// entirely separate from the server-side Operation vocabulary — see
// program.UIAction's doc comment.
type UIAction interface {
	isUIAction()
}

// SetLocalStateAction assigns the result of Value to Target within the
// mounted view's own client-local state. The compiler guarantees
// Target roots at "local".
type SetLocalStateAction struct {
	Target AssignmentTarget
	Value  Expression
}

func (SetLocalStateAction) isUIAction() {}

// AnswerQuestionAction submits the result of Value as the answer to
// the question instance currently active for the mounted view. Valid
// only in a view reached through a QuestionPresentation — the compiler
// rejects a View used from a plain Presentation whose Root contains
// this action anywhere.
type AnswerQuestionAction struct {
	Value Expression
}

func (AnswerQuestionAction) isUIAction() {}

// EmitUserIntentAction submits one instance of the named
// program.UserIntentDeclaration Intent, with Arguments evaluated in
// declaration order. The compiler guarantees Intent names a declared
// user intent and Arguments matches its declared parameters exactly.
type EmitUserIntentAction struct {
	Intent    string
	Arguments []CallArgument
}

func (EmitUserIntentAction) isUIAction() {}
