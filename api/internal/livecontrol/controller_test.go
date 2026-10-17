package livecontrol

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
			controller := New()
			for range tc.connections {
				require.True(t, controller.Go(tc.serve))
			}

			ctx, cancel := context.WithTimeout(context.Background(), tc.deadline)
			defer cancel()
			err := controller.Shutdown(ctx)

			require.ErrorIs(t, err, tc.expectedErr)
			require.False(t, controller.Go(func(context.Context) { t.Error("connection started after shutdown") }))
		})
	}
}
