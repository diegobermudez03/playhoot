package session

import "time"

// createSessionRequest/createSessionResponse are POST /sessions's wire
// shapes. The creator is never part of the request: it is the caller Identity
// resolved from the bearer token.
type createSessionRequest struct {
	GameUUID       string `json:"game_uuid"`
	IdempotencyKey string `json:"idempotency_key"`
}

type createSessionResponse struct {
	SessionUUID    string    `json:"session_uuid"`
	JoinCode       string    `json:"join_code"`
	LobbyExpiresAt time.Time `json:"lobby_expires_at"`
}
