package repo

import "time"

type session struct {
	ID                 uint       // PK, incremental
	UUID               string     // Exposed PK, uuid, INDEX UNIQUE
	GameDefinitionUUID string     // References the game definition UUID (external domain reference), INDEX
	OwnerUUID          string     // user uuid of the user who created the room session, INDEX
	StartedAt          time.Time  // Date in which the session was started, not null, always is the same created date
	EndedAt            *time.Time // Date in which the session was completed, NULL till completed
	CreatedAt          time.Time
}

type sessionState struct {
	ID          uint   // PK, incremental
	StateNumber uint   // incremental, source of truth for ordering (created at is just for data trailing), prevents collisions in case of concurrency
	SessionID   uint   // References same domain sessions.ID. UNIQUE INDEX(session_id, state_number)
	JSONState   string // JSON content of the state
	CreatedAt   time.Time
}

type sessionPlayer struct {
	ID         uint
	SessionID  string // references sessions table, INDEX
	PlayerUUID string // uuid of the player user
	JoinedAt   time.Time
	LeftAt     *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type joinCode struct {
	ID        uint
	Code      uint   // range 1000-9999
	SessionID string // references sessions table, INDEX
	CreatedAt time.Time
	DeletedAt *time.Time // deleted when the session starts or is cancelled or expired
}
