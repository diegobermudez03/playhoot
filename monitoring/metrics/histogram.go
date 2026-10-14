package metrics

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Histogram records the distribution of observations, such as latencies or
// payload sizes, into fixed buckets.
type Histogram struct {
	instrument
	buckets []float64
}

// LatencyBuckets returns bucket upper bounds, in seconds, suited to request
// and execution latencies from 5 ms to 10 s.
func LatencyBuckets() []float64 {
	return []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
}

// NewHistogram declares a histogram with the given bucket upper bounds, which
// must be non-empty, finite and strictly increasing. Name duration histograms
// with a _seconds suffix, as ObserveDuration records seconds.
func NewHistogram(name string, help string, buckets []float64, labels ...Label) *Histogram {
	if len(buckets) == 0 {
		panic(fmt.Sprintf("metrics: histogram %q declares no buckets", name))
	}
	for index, bound := range buckets {
		if math.IsNaN(bound) || math.IsInf(bound, 0) {
			panic(fmt.Sprintf("metrics: histogram %q has a non-finite bucket", name))
		}
		if index > 0 && bound <= buckets[index-1] {
			panic(fmt.Sprintf("metrics: histogram %q buckets must be strictly increasing", name))
		}
	}
	return &Histogram{
		instrument: newInstrument(kindHistogram, name, help, labels),
		buckets:    append([]float64(nil), buckets...),
	}
}

// Observe records one observation.
func (h *Histogram) Observe(ctx context.Context, value float64, values ...LabelValue) {
	h.record(ctx, operationObserve, value, values)
}

// ObserveDuration records elapsed as seconds.
func (h *Histogram) ObserveDuration(ctx context.Context, elapsed time.Duration, values ...LabelValue) {
	h.record(ctx, operationObserve, elapsed.Seconds(), values)
}
