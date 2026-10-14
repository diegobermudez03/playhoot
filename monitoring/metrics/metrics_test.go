package metrics

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/stretchr/testify/require"
)

func TestNewLabel(t *testing.T) {
	tests := map[string]struct {
		name        string
		values      []string
		expectPanic bool
	}{
		"valid":             {name: "result", values: []string{"accepted", "rejected"}},
		"uppercase_name":    {name: "Result", values: []string{"a"}, expectPanic: true},
		"name_with_dash":    {name: "join-result", values: []string{"a"}, expectPanic: true},
		"empty_name":        {name: "", values: []string{"a"}, expectPanic: true},
		"reserved_le":       {name: "le", values: []string{"a"}, expectPanic: true},
		"reserved_quantile": {name: "quantile", values: []string{"a"}, expectPanic: true},
		"no_values":         {name: "result", expectPanic: true},
		"empty_value":       {name: "result", values: []string{"a", ""}, expectPanic: true},
		"duplicated_value":  {name: "result", values: []string{"a", "a"}, expectPanic: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.expectPanic {
				require.Panics(t, func() { NewLabel(test.name, test.values...) })
				return
			}
			require.NotPanics(t, func() { NewLabel(test.name, test.values...) })
		})
	}
}

func TestLabelValue(t *testing.T) {
	label := NewLabel("result", "accepted", "rejected")

	require.NotPanics(t, func() { label.Value("accepted") })
	require.Panics(t, func() { label.Value("some_user_input") })
}

func TestNewInstrument(t *testing.T) {
	label := NewLabel("instrument_label", "a", "b")
	declareCounter(t, "instrument_taken_total", label)

	manyValues := func(count int) []string {
		values := make([]string, count)
		for i := range values {
			values[i] = "v" + strconv.Itoa(i)
		}
		return values
	}

	tests := map[string]struct {
		build       func() *Counter
		expectPanic bool
	}{
		"valid": {
			build: func() *Counter { return NewCounter("instrument_valid_total", "Help.", label) },
		},
		"invalid_name": {
			build:       func() *Counter { return NewCounter("Instrument-Bad", "Help.") },
			expectPanic: true,
		},
		"missing_help": {
			build:       func() *Counter { return NewCounter("instrument_nohelp_total", "") },
			expectPanic: true,
		},
		"duplicate_name": {
			build:       func() *Counter { return NewCounter("instrument_taken_total", "Help.") },
			expectPanic: true,
		},
		"zero_value_label": {
			build:       func() *Counter { return NewCounter("instrument_zero_label_total", "Help.", Label{}) },
			expectPanic: true,
		},
		"same_label_twice": {
			build:       func() *Counter { return NewCounter("instrument_twice_total", "Help.", label, label) },
			expectPanic: true,
		},
		"within_series_limit": {
			build: func() *Counter {
				return NewCounter("instrument_ok_series_total", "Help.",
					NewLabel("series_a", manyValues(10)...), NewLabel("series_b", manyValues(100)...))
			},
		},
		"over_series_limit": {
			build: func() *Counter {
				return NewCounter("instrument_big_series_total", "Help.",
					NewLabel("series_c", manyValues(10)...), NewLabel("series_d", manyValues(101)...))
			},
			expectPanic: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.expectPanic {
				require.Panics(t, func() { test.build() })
				return
			}
			require.NotPanics(t, func() { test.build() })
		})
	}
}

func TestResolveLabelValues(t *testing.T) {
	result := NewLabel("resolve_result", "accepted", "rejected")
	role := NewLabel("resolve_role", "player", "admin")
	other := NewLabel("resolve_other", "x")
	counter := declareCounter(t, "resolve_total", result, role)

	accepted, rejected := result.Value("accepted"), result.Value("rejected")
	player := role.Value("player")

	tests := map[string]struct {
		values      []LabelValue
		expected    []string
		expectPanic bool
	}{
		"declared_order":   {values: []LabelValue{accepted, player}, expected: []string{"accepted", "player"}},
		"reversed_order":   {values: []LabelValue{player, rejected}, expected: []string{"rejected", "player"}},
		"missing_value":    {values: []LabelValue{accepted}, expectPanic: true},
		"extra_value":      {values: []LabelValue{accepted, player, other.Value("x")}, expectPanic: true},
		"foreign_label":    {values: []LabelValue{accepted, other.Value("x")}, expectPanic: true},
		"same_label_twice": {values: []LabelValue{accepted, rejected}, expectPanic: true},
		"no_values":        {expectPanic: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.expectPanic {
				require.Panics(t, func() { counter.resolve(test.values) })
				return
			}
			require.Equal(t, test.expected, counter.resolve(test.values))
		})
	}
}

func TestMetricValueValidation(t *testing.T) {
	ctx := context.Background()
	counter := declareCounter(t, "validation_counter_total")
	gauge := NewGauge("validation_gauge", "Help.")
	histogram := NewHistogram("validation_duration_seconds", "Help.", LatencyBuckets())

	require.NotPanics(t, func() { counter.Inc(ctx) })
	require.NotPanics(t, func() { counter.Add(ctx, 2.5) })
	require.Panics(t, func() { counter.Add(ctx, -1) })
	require.Panics(t, func() { counter.Add(ctx, math.NaN()) })

	require.NotPanics(t, func() { gauge.Set(ctx, 3) })
	require.NotPanics(t, func() { gauge.Inc(ctx) })
	require.NotPanics(t, func() { gauge.Dec(ctx) })
	require.NotPanics(t, func() { gauge.Add(ctx, -4) })
	require.Panics(t, func() { gauge.Set(ctx, math.Inf(1)) })

	require.NotPanics(t, func() { histogram.Observe(ctx, 0.2) })
	require.NotPanics(t, func() { histogram.ObserveDuration(ctx, 150*time.Millisecond) })
	require.Panics(t, func() { histogram.Observe(ctx, math.NaN()) })
}

func TestNewHistogramBuckets(t *testing.T) {
	tests := map[string]struct {
		buckets     []float64
		expectPanic bool
	}{
		"increasing":     {buckets: []float64{0.1, 1, 10}},
		"single":         {buckets: []float64{1}},
		"empty":          {expectPanic: true},
		"equal_bounds":   {buckets: []float64{1, 1}, expectPanic: true},
		"decreasing":     {buckets: []float64{2, 1}, expectPanic: true},
		"infinite_bound": {buckets: []float64{1, math.Inf(1)}, expectPanic: true},
	}
	i := 0
	for name, test := range tests {
		i++
		metricName := "buckets_histogram_" + strconv.Itoa(i)
		t.Run(name, func(t *testing.T) {
			if test.expectPanic {
				require.Panics(t, func() { NewHistogram(metricName, "Help.", test.buckets) })
				return
			}
			require.NotPanics(t, func() { NewHistogram(metricName, "Help.", test.buckets) })
		})
	}
}

func TestRecordWritesToRequestLog(t *testing.T) {
	result := NewLabel("logged_result", "accepted", "rejected")
	counter := declareCounter(t, "logged_total", result)

	ctx := logging.Start(context.Background())
	counter.Inc(ctx, result.Value("accepted"))

	var output bytes.Buffer
	logging.FinishRequestLog(ctx, slog.New(slog.NewJSONHandler(&output, nil)), "test")

	require.Contains(t, output.String(), `"metric":"logged_total"`)
	require.Contains(t, output.String(), `"logged_result":"accepted"`)

	require.NotPanics(t, func() { counter.Inc(context.Background(), result.Value("rejected")) })
}

func declareCounter(t *testing.T, name string, labels ...Label) *Counter {
	t.Helper()
	return NewCounter(name, "Help.", labels...)
}
