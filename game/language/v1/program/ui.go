package program

// UILayout identifies how a ContainerElement arranges its children.
//
// UILayout is a closed interface. Its marker method is unexported so that
// packages outside program cannot introduce unsupported variants; the
// future compiler can safely exhaust all cases with a type switch.
type UILayout interface {
	isUILayout()
}

// StackLayout places children in the same logical region, allowing them to
// visually overlap. Child order determines the default stacking order
// unless an explicit property such as "z_index" overrides it.
type StackLayout struct{}

func (StackLayout) isUILayout() {}

// AbsoluteLayout positions each child independently through that child's
// own properties (for example "x", "y", "anchor_x", "anchor_y", "width",
// or "height"). The layout itself holds no child-position map; every
// position is declared on the child element.
type AbsoluteLayout struct{}

func (AbsoluteLayout) isUILayout() {}

// LinearLayoutDirection identifies the axis a LinearLayout arranges
// children along.
type LinearLayoutDirection string

const (
	// LinearLayoutDirectionRow arranges children horizontally.
	LinearLayoutDirectionRow LinearLayoutDirection = "row"

	// LinearLayoutDirectionColumn arranges children vertically.
	LinearLayoutDirectionColumn LinearLayoutDirection = "column"
)

// IsValid reports whether d is one of the linear-layout directions
// supported by this package.
func (d LinearLayoutDirection) IsValid() bool {
	switch d {
	case LinearLayoutDirectionRow, LinearLayoutDirectionColumn:
		return true
	default:
		return false
	}
}

// LinearLayout arranges children one after another along Direction. Gap is
// an optional numeric expression giving the spacing between consecutive
// children; a nil Gap means the renderer's default spacing applies.
// Alignment and justification are expected to be expressed through
// container properties rather than additional LinearLayout fields.
type LinearLayout struct {
	Direction LinearLayoutDirection
	Gap       Expression
}

func (LinearLayout) isUILayout() {}

// GridLayout arranges children into a grid. Columns must eventually
// compile to a positive integer number. RowGap and ColumnGap are optional
// numeric expressions giving row and column spacing; a nil gap means the
// renderer's default spacing applies. This package does not support
// responsive breakpoints or named grid areas.
type GridLayout struct {
	Columns   Expression
	RowGap    Expression
	ColumnGap Expression
}

func (GridLayout) isUILayout() {}

// UIElement is a node of a view's declarative UI element tree.
//
// The element tree, and every property or expression it contains, is pure
// with respect to model, local state, and repeat lexical bindings — only
// UIEventHandler actions may produce client actions. A UI element tree
// never contains server-side Block, Operation, or WorkflowControl values;
// UI actions and server operations are separate closed vocabularies.
//
// UIElement is a closed interface. Its marker method is unexported so
// that packages outside program cannot introduce unsupported variants;
// the future compiler can safely exhaust all cases with a type switch.
type UIElement interface {
	isUIElement()
}

// EmptyElement renders nothing.
//
// It is the explicit source-language representation of an intentionally
// empty branch — for example, one arm of a ConditionalElement, or a view
// that renders no content in some state. A nil UIElement may exist in a
// partially constructed, invalid source object, but it is not a valid way
// to represent "nothing"; the future compiler must reject it, and
// EmptyElement must be used instead.
type EmptyElement struct{}

func (EmptyElement) isUIElement() {}

// ContainerElement groups Children under one explicit Layout.
//
// Child order is semantically significant: it determines default
// rendering and stacking order for layouts such as StackLayout and the
// arrangement order for layouts such as LinearLayout. An empty Children
// slice is valid at the source-model level.
type ContainerElement struct {
	Configuration UIElementConfiguration
	Layout        UILayout
	Children      []UIElement
}

func (ContainerElement) isUIElement() {}

// TextElement renders Value as text. Value must eventually compile to the
// built-in string type. Text formatting (for example, interpolating a
// round number into a template) should be expressed through pure function
// calls in Value rather than a dedicated interpolation syntax.
type TextElement struct {
	Configuration UIElementConfiguration
	Value         Expression
}

func (TextElement) isUIElement() {}

// ImageElement renders an image identified by Source.
//
// The exact type Source must compile to is defined by the future compiler
// and client contract; this package represents it only as an Expression,
// with no dedicated built-in asset type. AlternativeText is an optional
// expression describing the image for accessibility purposes; a nil
// AlternativeText means no alternative text was authored, and the future
// compiler may require one in contexts where accessibility rules demand
// it.
type ImageElement struct {
	Configuration   UIElementConfiguration
	Source          Expression
	AlternativeText Expression
}

func (ImageElement) isUIElement() {}

// ButtonElement is an interactive visual container whose visible content
// is its Children (for example a TextElement, an ImageElement, or both).
// Click and other pointer behavior is represented through
// Configuration.Events, not a dedicated click field; enabled/visible
// behavior should be represented through typed properties in Configuration
// (such as "enabled" or "visible") rather than dedicated struct fields.
type ButtonElement struct {
	Configuration UIElementConfiguration
	Children      []UIElement
}

func (ButtonElement) isUIElement() {}

// RepeatElement declaratively renders one instance of Body for every value
// produced by Collection.
//
// Collection is evaluated from the current render inputs and must
// eventually compile to a finite list; iteration follows the list's order.
// ItemName is an immutable lexical binding for the current element, scoped
// to Body; IndexName is an optional immutable, zero-based numeric index
// binding, with an empty IndexName meaning no index binding is created.
// Neither binding escapes Body. Key is an optional, stable-identity
// expression evaluated within the repeated scope; a nil Key means no
// explicit key was authored, and the future compiler or renderer may
// require one in specific situations. This package does not support
// iterating a map directly, and the UI model has no imperative loop
// construct — RepeatElement is the only repetition mechanism.
type RepeatElement struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Key        Expression
	Body       UIElement
}

func (RepeatElement) isUIElement() {}

// ConditionalElement declaratively renders exactly one of Then or Else
// depending on Condition, which must eventually compile to a boolean
// value. Use EmptyElement for a branch that intentionally renders nothing.
// Branch selection is a pure rendering decision; it never executes
// authoritative game logic.
type ConditionalElement struct {
	Condition Expression
	Then      UIElement
	Else      UIElement
}

func (ConditionalElement) isUIElement() {}

// UIElementConfiguration groups the statically named properties and client
// event handlers attached to a UI element.
type UIElementConfiguration struct {
	Properties []UIProperty
	Events     []UIEventHandler
}

// UIProperty assigns Value to the statically named renderer capability
// Name (for example "width", "opacity", or "background_color").
//
// Property names remain plain strings, and Value remains a fully typed
// Expression, because different element types expose different property
// schemas and the supported property vocabulary is expected to grow
// independently of this structural AST; this package does not enumerate
// the complete property vocabulary. Properties are stored as a slice
// rather than a map so that source order, and duplicate property names,
// remain representable for the future compiler's deterministic
// diagnostics — this package does not validate whether a property is
// supported by its element type, its expected value type, or whether it
// is duplicated.
type UIProperty struct {
	Name  string
	Value Expression
}

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
