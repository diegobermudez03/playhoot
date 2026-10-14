package metrics

import "context"

// Gauge is a value that can go up and down, such as the number of active
// sessions or the depth of a queue.
type Gauge struct {
	instrument
}

// NewGauge declares a gauge.
func NewGauge(name string, help string, labels ...Label) *Gauge {
	return &Gauge{instrument: newInstrument(kindGauge, name, help, labels, nil)}
}

// Set replaces the current value.
func (g *Gauge) Set(ctx context.Context, value float64, values ...LabelValue) {
	g.record(ctx, operationSet, value, values)
}

// Add changes the current value by delta, which may be negative.
func (g *Gauge) Add(ctx context.Context, delta float64, values ...LabelValue) {
	g.record(ctx, operationAdd, delta, values)
}

// Inc adds one.
func (g *Gauge) Inc(ctx context.Context, values ...LabelValue) {
	g.record(ctx, operationAdd, 1, values)
}

// Dec subtracts one.
func (g *Gauge) Dec(ctx context.Context, values ...LabelValue) {
	g.record(ctx, operationAdd, -1, values)
}
