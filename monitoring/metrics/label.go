package metrics

import (
	"fmt"
	"regexp"
)

var labelNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// reservedLabelNames are added or interpreted by common metric systems for
// histograms, so a user-declared label with one of these names would collide.
var reservedLabelNames = map[string]struct{}{
	"le":       {},
	"quantile": {},
}

type labelDef struct {
	name   string
	values []string
	index  map[string]struct{}
}

// Label is a metric dimension with a closed set of allowed values. Closing
// the set at declaration is what bounds a metric's cardinality: a caller can
// never record a value that was not declared up front.
//
// A Label is declared once, by the package that owns the metric, and can be
// shared by several metrics.
type Label struct {
	def *labelDef
}

// LabelValue is one declared value of a Label. It can only be obtained from
// Label.Value, so a metric call can never receive an arbitrary string.
type LabelValue struct {
	def   *labelDef
	value string
}

// NewLabel declares a label called name whose only allowed values are values.
// It panics on an invalid or reserved name, on no values, and on empty or
// duplicated values, since all of those are programming errors that must
// surface at startup.
func NewLabel(name string, values ...string) Label {
	if !labelNamePattern.MatchString(name) {
		panic(fmt.Sprintf("metrics: invalid label name %q, must be lower snake_case", name))
	}
	if _, reserved := reservedLabelNames[name]; reserved {
		panic(fmt.Sprintf("metrics: label name %q is reserved", name))
	}
	if len(values) == 0 {
		panic(fmt.Sprintf("metrics: label %q declares no values", name))
	}

	index := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			panic(fmt.Sprintf("metrics: label %q declares an empty value", name))
		}
		if _, duplicate := index[value]; duplicate {
			panic(fmt.Sprintf("metrics: label %q declares value %q twice", name, value))
		}
		index[value] = struct{}{}
	}

	return Label{def: &labelDef{
		name:   name,
		values: append([]string(nil), values...),
		index:  index,
	}}
}

// Name returns the label's name.
func (l Label) Name() string {
	return l.def.name
}

// Value returns the LabelValue for value. It panics when value was not
// declared in NewLabel. Resolve values once, where the metric is declared,
// rather than per call, so an undeclared value fails at startup instead of on
// a live request.
func (l Label) Value(value string) LabelValue {
	if _, ok := l.def.index[value]; !ok {
		panic(fmt.Sprintf("metrics: value %q is not declared for label %q", value, l.def.name))
	}
	return LabelValue{def: l.def, value: value}
}
