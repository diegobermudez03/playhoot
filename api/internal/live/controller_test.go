package live

import (
	"context"
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
