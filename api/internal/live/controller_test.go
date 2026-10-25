package live

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShutdown(t *testing.T) {
	type test struct {
		// serve is what each running connection does once told to end.
		serve       func(shutdown context.Context)
		connections int
		deadline    time.Duration

		expectedErr error
	}

	tests := map[string]func(t *testing.T) test{
		"connections_that_end_when_told_are_waited_for": func(t *testing.T) test {
			return test{
				serve:       func(shutdown context.Context) { <-shutdown.Done() },
				connections: 3,
				deadline:    time.Second,
			}
		},
		"no_connections": func(t *testing.T) test {
			return test{deadline: time.Second}
		},
		"connection_that_does_not_end_in_time_reports_the_deadline": func(t *testing.T) test {
			return test{
				serve: func(shutdown context.Context) {
					<-shutdown.Done()
					time.Sleep(time.Second)
				},
				connections: 1,
				deadline:    20 * time.Millisecond,
				expectedErr: context.DeadlineExceeded,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller := New(0)
			for range tc.connections {
				require.True(t, controller.start(tc.serve))
			}

			ctx, cancel := context.WithTimeout(context.Background(), tc.deadline)
			defer cancel()
			err := controller.Shutdown(ctx)

			require.ErrorIs(t, err, tc.expectedErr)
			require.False(t, controller.start(func(context.Context) { t.Error("connection started after shutdown") }))
		})
	}
}

func TestReserve(t *testing.T) {
	type reservation struct {
		caller string
		// release gives the room back right after taking it.
		release bool

		expectedErr error
	}
	type test struct {
		maxConnections int
		reservations   []reservation
	}

	repeat := func(n int, r reservation) []reservation {
		reservations := make([]reservation, n)
		for i := range reservations {
			reservations[i] = r
		}
		return reservations
	}

	tests := map[string]func(t *testing.T) test{
		"an_instance_full_refuses_every_caller": func(t *testing.T) test {
			return test{
				maxConnections: 2,
				reservations: []reservation{
					{caller: "a"},
					{caller: "b"},
					{caller: "c", expectedErr: ErrInstanceFull},
				},
			}
		},
		"a_caller_with_as_many_as_allowed_is_refused_while_others_are_not": func(t *testing.T) test {
			return test{
				reservations: append(repeat(MaxConnectionsPerCaller, reservation{caller: "a"}),
					reservation{caller: "a", expectedErr: ErrCallerFull},
					reservation{caller: "b"},
				),
			}
		},
		"released_room_is_taken_again": func(t *testing.T) test {
			return test{
				maxConnections: 1,
				reservations: []reservation{
					{caller: "a", release: true},
					{caller: "b", release: true},
					{caller: "a"},
					{caller: "c", expectedErr: ErrInstanceFull},
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller := New(tc.maxConnections)
			for i, r := range tc.reservations {
				release, err := controller.Reserve(context.Background(), r.caller)
				require.ErrorIs(t, err, r.expectedErr, "reservation %d", i)
				if r.release {
					release()
					// Releasing twice gives back nothing more.
					release()
				}
			}
		})
	}
}

func TestReserveRefusal(t *testing.T) {
	type refusal struct {
		caller string
		// release gives the room back right after taking it.
		release bool

		expectedOK bool
	}
	type test struct {
		// connections are reserved first, filling the instance.
		connections []string
		refusals    []refusal
	}

	tests := map[string]func(t *testing.T) test{
		"a_caller_is_refused_on_at_most_its_own_share_of_connections": func(t *testing.T) test {
			refusals := make([]refusal, 0, MaxRefusalsPerCaller+2)
			for range MaxRefusalsPerCaller {
				refusals = append(refusals, refusal{caller: "a", expectedOK: true})
			}
			return test{refusals: append(refusals, refusal{caller: "a"}, refusal{caller: "b", expectedOK: true})}
		},
		"the_instance_refuses_on_at_most_its_bound_of_connections": func(t *testing.T) test {
			refusals := make([]refusal, 0, MaxRefusals+1)
			for i := range MaxRefusals {
				refusals = append(refusals, refusal{caller: strconv.Itoa(i), expectedOK: true})
			}
			return test{refusals: append(refusals, refusal{caller: "late"})}
		},
		"a_full_instance_still_has_room_to_refuse": func(t *testing.T) test {
			return test{connections: []string{"a"}, refusals: []refusal{{caller: "b", expectedOK: true}}}
		},
		"released_room_is_taken_again": func(t *testing.T) test {
			refusals := make([]refusal, 0, MaxRefusalsPerCaller+1)
			for range MaxRefusalsPerCaller + 1 {
				refusals = append(refusals, refusal{caller: "a", release: true, expectedOK: true})
			}
			return test{refusals: refusals}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller := New(max(1, len(tc.connections)))
			for _, caller := range tc.connections {
				_, err := controller.Reserve(context.Background(), caller)
				require.NoError(t, err)
			}
			for i, r := range tc.refusals {
				release, ok := controller.ReserveRefusal(context.Background(), r.caller)
				require.Equal(t, r.expectedOK, ok, "refusal %d", i)
				if r.release {
					release()
					// Releasing twice gives back nothing more.
					release()
				}
			}
			if len(tc.connections) > 0 {
				_, err := controller.Reserve(context.Background(), "other")
				require.ErrorIs(t, err, ErrInstanceFull, "refusing takes none of the connections' room")
			}
		})
	}
}
