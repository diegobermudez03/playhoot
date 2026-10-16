package metrics

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
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

// Label is a metric dimension with a closed set of allowed values, which is
// what bounds a metric's cardinality. It can be shared by several metrics.
type Label struct {
	def *labelDef
}

// LabelValue is one declared value of a Label. It can only be obtained from
// Label.Value, so a metric call can never receive an arbitrary string.
type LabelValue struct {
	def   *labelDef
	value string
}

// labelRegistry interns labels by their name and set of values, so declaring
// the same label again yields the same label. This is what lets metrics
// declared from code that can run more than once (for example, when a server
// is built) be redeclared without conflict.
var labelRegistry = struct {
	mu     sync.Mutex
	labels map[string]*labelDef
}{labels: map[string]*labelDef{}}

// NewLabel declares a label whose only allowed values are values; redeclaring
// it identically returns the same label. Invalid declarations panic.
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

	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	key := name + "\x00" + strings.Join(sorted, "\x00")

	labelRegistry.mu.Lock()
	defer labelRegistry.mu.Unlock()
	if existing, ok := labelRegistry.labels[key]; ok {
		return Label{def: existing}
	}
	def := &labelDef{
		name:   name,
		values: append([]string(nil), values...),
		index:  index,
	}
	labelRegistry.labels[key] = def
	return Label{def: def}
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
