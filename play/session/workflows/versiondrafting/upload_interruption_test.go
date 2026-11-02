package versiondrafting

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadInterruptionCause(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := map[string]struct {
		ctx      context.Context
		err      error
		expected string
	}{
		"read_deadline":      {ctx: context.Background(), err: fmt.Errorf("read: %w", os.ErrDeadlineExceeded), expected: "read_timeout"},
		"truncated_body":     {ctx: context.Background(), err: io.ErrUnexpectedEOF, expected: "client_gone"},
		"connection_reset":   {ctx: context.Background(), err: fmt.Errorf("read: %w", syscall.ECONNRESET), expected: "client_gone"},
		"request_cancelled":  {ctx: cancelled, err: errors.New("body closed"), expected: "client_gone"},
		"other_read_failure": {ctx: context.Background(), err: errors.New("disk full"), expected: "read_failed"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, readInterruptionCause(tc.ctx, tc.err))
		})
	}
}
