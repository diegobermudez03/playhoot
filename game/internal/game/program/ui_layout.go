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
