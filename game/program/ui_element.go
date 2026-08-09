package program

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
