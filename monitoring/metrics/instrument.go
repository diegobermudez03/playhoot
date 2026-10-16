// Package metrics records counters, gauges and histograms with closed-set
// labels. Owning packages declare their own metrics; redeclaring identically
// returns the existing one, and misuse panics because cardinality must stay
// bounded.
package metrics

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sync"

	"github.com/diegobermudez03/playhoot/logging"
)

// maxSeriesPerMetric bounds the number of distinct label combinations one
// metric can produce, so that several individually small labels cannot
// multiply into an unbounded number of series.
const maxSeriesPerMetric = 1000

var metricNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var registry = struct {
	mu          sync.Mutex
	instruments map[string]instrument
}{instruments: map[string]instrument{}}

type kind string

const (
	kindCounter   kind = "counter"
	kindGauge     kind = "gauge"
	kindHistogram kind = "histogram"
)

type operation string

const (
	operationAdd     operation = "add"
	operationSet     operation = "set"
	operationObserve operation = "observe"
)

// instrument holds what every metric kind shares: its identity, its labels,
// and the recording path.
type instrument struct {
	name    string
	help    string
	kind    kind
	labels  []Label
	buckets []float64
}

// sameDefinition reports whether other declares exactly the same metric.
// Labels are interned, so comparing them by identity compares their values.
func (i instrument) sameDefinition(other instrument) bool {
	if i.name != other.name || i.help != other.help || i.kind != other.kind ||
		len(i.labels) != len(other.labels) || len(i.buckets) != len(other.buckets) {
		return false
	}
	for index := range i.labels {
		if i.labels[index].def != other.labels[index].def {
			return false
		}
	}
	for index := range i.buckets {
		if i.buckets[index] != other.buckets[index] {
			return false
		}
	}
	return true
}

// newInstrument validates the declaration and reserves its name. Metric names
// are global to the process: declaring a metric again with exactly the same
// definition returns the existing one, while declaring the same name with a
// different definition panics. buckets is only used by histograms.
func newInstrument(kind kind, name string, help string, labels []Label, buckets []float64) instrument {
	if !metricNamePattern.MatchString(name) {
		panic(fmt.Sprintf("metrics: invalid metric name %q, must be lower snake_case", name))
	}
	if help == "" {
		panic(fmt.Sprintf("metrics: metric %q has no help text", name))
	}

	series := 1
	seen := make(map[*labelDef]struct{}, len(labels))
	for _, label := range labels {
		if label.def == nil {
			panic(fmt.Sprintf("metrics: metric %q uses an undeclared label, create it with NewLabel", name))
		}
		if _, duplicate := seen[label.def]; duplicate {
			panic(fmt.Sprintf("metrics: metric %q uses label %q twice", name, label.def.name))
		}
		seen[label.def] = struct{}{}
		series *= len(label.def.values)
		if series > maxSeriesPerMetric {
			panic(fmt.Sprintf("metrics: metric %q would exceed %d label combinations", name, maxSeriesPerMetric))
		}
	}

	declared := instrument{
		name:    name,
		help:    help,
		kind:    kind,
		labels:  append([]Label(nil), labels...),
		buckets: append([]float64(nil), buckets...),
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if existing, exists := registry.instruments[name]; exists {
		if !existing.sameDefinition(declared) {
			panic(fmt.Sprintf("metrics: metric %q is already declared with a different definition", name))
		}
		return existing
	}
	registry.instruments[name] = declared
	return declared
}

// resolve returns the label values in the metric's declared label order. The
// caller may pass them in any order, but must pass exactly one value for each
// of the metric's labels.
func (i instrument) resolve(values []LabelValue) []string {
	if len(values) != len(i.labels) {
		panic(fmt.Sprintf("metrics: metric %q takes %d label values, got %d", i.name, len(i.labels), len(values)))
	}

	resolved := make([]string, len(i.labels))
	filled := make([]bool, len(i.labels))
	for _, value := range values {
		position := -1
		for index, label := range i.labels {
			if label.def == value.def {
				position = index
				break
			}
		}
		if position == -1 {
			panic(fmt.Sprintf("metrics: metric %q does not have the label of value %q", i.name, value.value))
		}
		if filled[position] {
			panic(fmt.Sprintf("metrics: metric %q got label %q twice", i.name, i.labels[position].def.name))
		}
		filled[position] = true
		resolved[position] = value.value
	}
	return resolved
}

// record validates a measurement and emits it. Like monitoring.Alert, every
// call is also written to the request log carried by ctx, so a point on a
// dashboard can be matched with the logs of the requests that produced it.
func (i instrument) record(ctx context.Context, operation operation, amount float64, values []LabelValue) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		panic(fmt.Sprintf("metrics: metric %q recorded a non-finite value", i.name))
	}
	resolved := i.resolve(values)

	labels := make(map[string]string, len(resolved))
	for index, label := range i.labels {
		labels[label.def.name] = resolved[index]
	}
	logging.LogLoopFields(ctx, "metrics",
		logging.Field("metric", i.name),
		logging.Field("operation", string(operation)),
		logging.Field("value", amount),
		logging.Field("labels", labels),
	)
	// TODO: forward the measurement to the metrics backend once one is integrated.
}
