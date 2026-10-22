package live

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// Handler handles one client message of a lane's type; it ends the connection
// the same way a Loop does.
type Handler func(ctx context.Context, data []byte) (end End, ended bool)

// LaneLimits bound a lane: how often its messages may arrive and how many may
// wait to be handled. A message beyond them goes to Excess, on the reading
// goroutine, or is dropped when Excess is nil.
type LaneLimits struct {
	Rate     []ratelimit.Limit
	Capacity int
	Excess   Handler
	// Refused, when set, is told every message these limits refuse, by limit:
	// "rate" or "queue".
	Refused func(ctx context.Context, limit string)
}

// ConnectionLimits bound everything a client sends on one connection, all
// message types together.
type ConnectionLimits struct {
	Messages []ratelimit.Limit
	Bytes    []ratelimit.Limit
	// Refused, when set, is told the limit that ended the connection:
	// "messages" or "bytes".
	Refused func(ctx context.Context, limit string)
}

// Lane handles one message type's messages in order, on its own goroutine, so a
// slow message only delays later ones of its type.
type Lane struct {
	handle   Handler
	excess   Handler
	refused  func(ctx context.Context, limit string)
	rate     *ratelimit.Limiter
	capacity int

	mu      sync.Mutex
	pending [][]byte
	// wake holds at most one signal that pending grew.
	wake chan struct{}
}

// NewLane constructs a Lane handling each message with handle within limits.
// It panics without a capacity, which is a programming error.
func NewLane(handle Handler, limits LaneLimits) *Lane {
	if limits.Capacity < 1 {
		panic(fmt.Sprintf("live: lane capacity must be positive, got %d", limits.Capacity))
	}
	lane := &Lane{handle: handle, excess: limits.Excess, refused: limits.Refused, capacity: limits.Capacity, wake: make(chan struct{}, 1)}
	if len(limits.Rate) > 0 {
		lane.rate = ratelimit.New(limits.Rate...)
	}
	return lane
}

// offer adds a message to the lane without ever blocking, or refuses it when
// the lane is full or its type arrives too often.
func (l *Lane) offer(ctx context.Context, data []byte, now time.Time) (End, bool) {
	l.mu.Lock()
	full := len(l.pending) >= l.capacity
	l.mu.Unlock()
	if full {
		return l.refuse(ctx, data, limitTypeQueue)
	}
	if l.rate != nil {
		if allowed, _ := l.rate.Allow(now); !allowed {
			return l.refuse(ctx, data, limitTypeRate)
		}
	}
	l.mu.Lock()
	l.pending = append(l.pending, data)
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
	return End{}, false
}

func (l *Lane) refuse(ctx context.Context, data []byte, limit string) (End, bool) {
	messagesRefused.Inc(ctx, limitLabel.Value(limit))
	if l.refused != nil {
		l.refused(ctx, strings.TrimPrefix(limit, "type_"))
	}
	if l.excess == nil {
		return End{}, false
	}
	return l.excess(ctx, data)
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
// its type's lane, never waiting for one to be handled. An unknown type, or a
// client sending more than limits allow, ends the connection.
func Dispatch(conn *websocket.Conn, lanes map[string]*Lane, limits ConnectionLimits) Loop {
	var messages, bytes *ratelimit.Limiter
	if len(limits.Messages) > 0 {
		messages = ratelimit.New(limits.Messages...)
	}
	if len(limits.Bytes) > 0 {
		bytes = ratelimit.New(limits.Bytes...)
	}
	return func(ctx context.Context) (End, bool) {
		for {
			messageType, data, end, ended := ReadMessage(ctx, conn)
			if ended || messageType == "" {
				return end, ended
			}
			now := time.Now()
			if messages != nil {
				if allowed, _ := messages.Allow(now); !allowed {
					messagesRefused.Inc(ctx, limitLabel.Value(limitConnectionMessages))
					if limits.Refused != nil {
						limits.Refused(ctx, "messages")
					}
					return CloseRateLimited(conn), true
				}
			}
			if bytes != nil {
				if allowed, _ := bytes.AllowN(now, len(data)); !allowed {
					messagesRefused.Inc(ctx, limitLabel.Value(limitConnectionBytes))
					if limits.Refused != nil {
						limits.Refused(ctx, "bytes")
					}
					return CloseRateLimited(conn), true
				}
			}
			lane, found := lanes[messageType]
			if !found {
				return CloseUnknownType(conn, messageType), true
			}
			if end, ended := lane.offer(ctx, data, now); ended {
				return end, ended
			}
		}
	}
}
