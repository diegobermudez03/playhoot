package live

import (
	"context"
	"sync"

	"github.com/coder/websocket"
)

// Handler handles one client message of a lane's type; it ends the connection
// the same way a Loop does.
type Handler func(ctx context.Context, data []byte) (end End, ended bool)

// Lane handles one message type's messages in order, on its own goroutine, so a
// slow message only delays later ones of its type.
type Lane struct {
	handle Handler

	mu      sync.Mutex
	pending [][]byte
	// wake holds at most one signal that pending grew.
	wake chan struct{}
}

// NewLane constructs a Lane handling each message with handle.
func NewLane(handle Handler) *Lane {
	return &Lane{handle: handle, wake: make(chan struct{}, 1)}
}

// enqueue adds a message to the lane without ever blocking; the queue is
// unbounded, so its growth is limited only by the client's message rate.
func (l *Lane) enqueue(data []byte) {
	l.mu.Lock()
	l.pending = append(l.pending, data)
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// Run is the lane's loop: it handles every enqueued message in order until one
// ends the connection or ctx is done.
func (l *Lane) Run(ctx context.Context) (End, bool) {
	for {
		data, ok := l.next(ctx)
		if !ok {
			return End{}, false
		}
		if end, ended := l.handle(ctx, data); ended || ctx.Err() != nil {
			return end, ended
		}
	}
}

func (l *Lane) next(ctx context.Context) ([]byte, bool) {
	for {
		l.mu.Lock()
		if len(l.pending) > 0 {
			data := l.pending[0]
			l.pending[0] = nil
			l.pending = l.pending[1:]
			l.mu.Unlock()
			return data, true
		}
		l.mu.Unlock()
		select {
		case <-l.wake:
		case <-ctx.Done():
			return nil, false
		}
	}
}

// Dispatch returns the loop reading the client's messages and handing each to
// its type's lane, never waiting for one to be handled. An unknown type ends
// the connection.
func Dispatch(conn *websocket.Conn, lanes map[string]*Lane) Loop {
	return func(ctx context.Context) (End, bool) {
		for {
			messageType, data, end, ended := ReadMessage(ctx, conn)
			if ended || messageType == "" {
				return end, ended
			}
			lane, found := lanes[messageType]
			if !found {
				return CloseUnknownType(conn, messageType), true
			}
			lane.enqueue(data)
		}
	}
}
