package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var epoch = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// event is one call to a Limiter: an Allow (or Record) after the previous one.
type event struct {
	after  time.Duration
	n      int
	record bool

	expectedAllowed    bool
	expectedRetryAfter time.Duration
}

func TestLimiterAllow(t *testing.T) {
	type test struct {
		limits []Limit
		events []event
	}

	tests := map[string]func(t *testing.T) test{
		"burst_up_to_the_count_then_refused_until_room_comes_back": func(t *testing.T) test {
			return test{
				limits: []Limit{{Count: 2, Per: time.Second}},
				events: []event{
					{expectedAllowed: true},
					{expectedAllowed: true},
					{expectedAllowed: false, expectedRetryAfter: 500 * time.Millisecond},
					{after: 500 * time.Millisecond, expectedAllowed: true},
				},
			}
		},
		"per_second_and_per_minute_both_apply": func(t *testing.T) test {
			// 3 a second but only 4 a minute: the fifth is refused by the
			// minute even after the second refilled.
			return test{
				limits: []Limit{{Count: 3, Per: time.Second}, {Count: 4, Per: time.Minute}},
				events: []event{
					{expectedAllowed: true},
					{expectedAllowed: true},
					{expectedAllowed: true},
					{after: time.Second, expectedAllowed: true},
					{after: time.Second, expectedAllowed: false, expectedRetryAfter: 13 * time.Second},
					{after: 13 * time.Second, expectedAllowed: true},
				},
			}
		},
		"a_refusal_takes_nothing_from_any_limit": func(t *testing.T) test {
			return test{
				limits: []Limit{{Count: 1, Per: time.Second}, {Count: 10, Per: time.Minute}},
				events: []event{
					{expectedAllowed: true},
					{expectedAllowed: false, expectedRetryAfter: time.Second},
					{expectedAllowed: false, expectedRetryAfter: time.Second},
					{after: time.Second, expectedAllowed: true},
				},
			}
		},
		"one_every_five_minutes": func(t *testing.T) test {
			return test{
				limits: []Limit{{Count: 1, Per: 5 * time.Minute}},
				events: []event{
					{expectedAllowed: true},
					{after: 4 * time.Minute, expectedAllowed: false, expectedRetryAfter: time.Minute},
					{after: time.Minute, expectedAllowed: true},
				},
			}
		},
		"n_events_at_once": func(t *testing.T) test {
			return test{
				limits: []Limit{{Count: 100, Per: time.Second}},
				events: []event{
					{n: 60, expectedAllowed: true},
					{n: 60, expectedAllowed: false, expectedRetryAfter: 200 * time.Millisecond},
					{n: 40, expectedAllowed: true},
				},
			}
		},
		"recorded_events_count_but_are_never_refused": func(t *testing.T) test {
			return test{
				limits: []Limit{{Count: 2, Per: time.Second}},
				events: []event{
					{record: true},
					{record: true},
					{record: true},
					{expectedAllowed: false, expectedRetryAfter: 500 * time.Millisecond},
				},
			}
		},
		"a_clock_going_backwards_adds_no_room": func(t *testing.T) test {
			return test{
				limits: []Limit{{Count: 1, Per: time.Second}},
				events: []event{
					{expectedAllowed: true},
					{after: -time.Hour, expectedAllowed: false, expectedRetryAfter: time.Second},
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			limiter := New(tc.limits...)
			now := epoch
			for i, e := range tc.events {
				now = now.Add(e.after)
				if e.record {
					limiter.Record(now)
					continue
				}
				n := e.n
				if n == 0 {
					n = 1
				}
				allowed, retryAfter := limiter.AllowN(now, n)
				require.Equal(t, e.expectedAllowed, allowed, "event %d", i)
				require.Equal(t, e.expectedRetryAfter, retryAfter, "event %d", i)
			}
		})
	}
}

func TestNewPanicsOnALimitAllowingNothing(t *testing.T) {
	require.Panics(t, func() { New(Limit{Count: 0, Per: time.Second}) })
	require.Panics(t, func() { NewKeyed(Limit{Count: 1, Per: 0}) })
}

func TestKeyedAllow(t *testing.T) {
	type call struct {
		key   string
		after time.Duration

		expectedAllowed bool
		// expectedSwept means the call also forgot the keys whose room came
		// back, which happens at most once per the longest limit.
		expectedSwept bool
	}
	type test struct {
		limits []Limit
		calls  []call

		expectedKeys int
	}

	tests := map[string]func(t *testing.T) test{
		"each_key_has_its_own_room": func(t *testing.T) test {
			return test{
				limits: []Limit{{Count: 1, Per: time.Second}},
				calls: []call{
					{key: "a", expectedAllowed: true, expectedSwept: true},
					{key: "a", expectedAllowed: false},
					{key: "b", expectedAllowed: true},
				},
				expectedKeys: 2,
			}
		},
		"keys_whose_room_came_back_are_forgotten": func(t *testing.T) test {
			return test{
				limits: []Limit{{Count: 1, Per: time.Second}, {Count: 5, Per: time.Minute}},
				calls: []call{
					{key: "a", expectedAllowed: true, expectedSwept: true},
					{key: "b", expectedAllowed: true},
					{key: "c", after: time.Minute, expectedAllowed: true, expectedSwept: true},
				},
				expectedKeys: 1,
			}
		},
		"keys_still_limited_are_kept": func(t *testing.T) test {
			return test{
				limits: []Limit{{Count: 1, Per: time.Minute}},
				calls: []call{
					{key: "a", expectedAllowed: true, expectedSwept: true},
					{key: "b", after: 59 * time.Second, expectedAllowed: true},
					{key: "c", after: 2 * time.Second, expectedAllowed: true, expectedSwept: true},
					{key: "b", expectedAllowed: false},
				},
				expectedKeys: 2,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			keyed := NewKeyed(tc.limits...)
			now := epoch
			for i, c := range tc.calls {
				now = now.Add(c.after)
				allowed, _, swept := keyed.Allow(c.key, now)
				require.Equal(t, c.expectedAllowed, allowed, "call %d", i)
				require.Equal(t, c.expectedSwept, swept, "call %d", i)
			}
			require.Equal(t, tc.expectedKeys, keyed.Len())
		})
	}
}
