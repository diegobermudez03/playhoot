package routing

import (
	"context"
	"log/slog"
	"sync"

	"github.com/diegobermudez03/playhoot/logging"
)

type handshakeLogContextKey struct{}

type handshakeLog struct {
	once    sync.Once
	ctx     context.Context
	message string
}

// StartHandshakeLog starts a WebSocket handshake's own request log as a child
// span of ctx.
func StartHandshakeLog(ctx context.Context, pattern string) context.Context {
	handshakeCtx := logging.Start(ctx)
	return context.WithValue(handshakeCtx, handshakeLogContextKey{}, &handshakeLog{ctx: handshakeCtx, message: pattern + " handshake"})
}

// FinishHandshakeLog flushes the handshake log; only the first call does.
func FinishHandshakeLog(ctx context.Context) {
	handshake, ok := ctx.Value(handshakeLogContextKey{}).(*handshakeLog)
	if !ok {
		return
	}
	handshake.once.Do(func() {
		logging.FinishRequestLog(handshake.ctx, slog.Default(), handshake.message)
	})
}
