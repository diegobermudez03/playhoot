// Package storage documents the Session Runtime persisted schema. These
// types are not used as GORM models by any query - actual repository code
// defines its own row-shaped structs local to each usecase package (see
// repositories.md) - they exist only to mirror the migrations below as
// readable schema documentation.
package storage

import "time"

type session struct {
	ID                 uint       // PK, incremental
	UUID               string     // Exposed PK, uuid, INDEX UNIQUE
	GameDefinitionUUID string     // Pinned Game Definition/Version UUID (external domain reference, no FK), resolved exactly once at Create
	HostActorID        *uint      // References session_actors.id; nullable at storage level only during the Host/SessionActor Creation Cycle
	Phase              string     // LOBBY | TERMINAL (this work never reaches RUNNING)
	LobbyExpiresAt     time.Time  // Authoritative LOBBY deadline
	StartedAt          *time.Time // Always NULL in this work; a later slice sets it on Start
	TerminalAt         *time.Time // NULL until phase = TERMINAL
	TerminalReason     *string    // NULL until phase = TERMINAL
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type sessionActor struct {
	ID               uint   // PK, incremental
	SessionID        uint   // References sessions.id, INDEX
	UserUUID         string // References Identity.User.user_uuid (external domain reference, no FK)
	SemanticPresence string // CONNECTED | DISCONNECTED (GAME-ADR-0015); this work only ever produces CONNECTED
	CreatedAt        time.Time
	// UNIQUE INDEX(session_id, user_uuid)
}

type sessionParticipant struct {
	ID             uint   // PK, incremental
	SessionActorID uint   // References session_actors.id, UNIQUE (at most one Participant per SessionActor)
	DisplayName    string // Session-scoped display-name snapshot
	Active         bool   // Currently occupies/admits a lobby slot
	JoinedAt       time.Time
	LeftAt         *time.Time // NULL while active
}

type joinCode struct {
	ID        uint // PK, incremental
	SessionID uint // References sessions.id, INDEX
	Code      uint // range 1000-9999; UNIQUE among rows where revoked_at IS NULL
	CreatedAt time.Time
	RevokedAt *time.Time // NULL while active; revoked (not deleted) when the lobby is no longer admissible; UNIQUE(session_id) among rows where revoked_at IS NULL
}

type sessionRequest struct {
	ID              uint    // PK, incremental
	Operation       string  // CREATE | JOIN | LEAVE (Slice 1 subset)
	IdempotencyKey  string  // Caller-supplied opaque key
	UserUUID        string  // References Identity.User.user_uuid (external domain reference, no FK)
	SessionID       *uint   // References sessions.id; NULL for a CREATE request with no resulting Session
	RequestPayload  string  // JSON; per-operation meaningful fields for conflict detection
	Outcome         string  // Short outcome label (e.g. "CREATED")
	ResponsePayload *string // JSON; stored result for idempotent replay
	Status          string  // PENDING | COMPLETED
	CreatedAt       time.Time
	// UNIQUE INDEX(user_uuid, operation, idempotency_key)
}
