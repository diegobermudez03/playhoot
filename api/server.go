// Package api is the external transport/application edge: it owns the
// HTTP upgrade handshake and one read-pump/write-pump goroutine pair per
// live connection, decodes/encodes wire JSON, and forwards decoded client
// commands to play - it holds no Session/connection registry and no
// business state of its own, and depends only on play's exported API,
// never on any `game` package.
package api

import "net/http"

// Server is the WebSocket/HTTP transport adapter in front of a
// play.Coordinator.
type Server struct {
	coord coordinatorAPI
}

// NewServer constructs a Server dispatching against coord.
func NewServer(coord coordinatorAPI) *Server {
	return &Server{coord: coord}
}

// Routes returns Server's HTTP handler: Create/Join as ordinary HTTP
// request/response, and the WebSocket upgrade endpoint that then carries
// Start/AnswerInteraction and their resulting fan-out.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sessions", s.handleCreateSession)
	mux.HandleFunc("POST /sessions/join", s.handleJoinSession)
	mux.HandleFunc("GET /ws", s.handleWebSocket)
	return mux
}
