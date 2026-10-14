package metrics

import (
	"context"
	"fmt"
)

// Counter is a value that only goes up, such as completed operations or
// errors. Name counters with a _total suffix.
type Counter struct {
	instrument
}

// NewCounter declares a counter. Pass one label per dimension; each recording
// call must then provide one value for each of them.
func NewCounter(name string, help string, labels ...Label) *Counter {
	return &Counter{instrument: newInstrument(kindCounter, name, help, labels, nil)}
}

// Inc adds one.
func (c *Counter) Inc(ctx context.Context, values ...LabelValue) {
	c.record(ctx, operationAdd, 1, values)
}

// Add adds delta, which must not be negative.
func (c *Counter) Add(ctx context.Context, delta float64, values ...LabelValue) {
	if delta < 0 {
		panic(fmt.Sprintf("metrics: counter %q cannot decrease", c.name))
	}
	c.record(ctx, operationAdd, delta, values)
}
