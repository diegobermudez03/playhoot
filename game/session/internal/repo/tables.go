package repo

import "time"

type session struct {
	ID              uint       // PK, incremental
	UUID            string     // Exposed PK, uuid
	GameVersionUUID string     // References the game version UUID (external domain reference), external domain can match with exact version and there main game associated
	StartedAt       time.Time  // Date in which the session was started, not null, always is the same created date
	EndedAt         *time.Time // Date in which the session was completed, NULL till completed
	CreatedAt       time.Time
}

type sessionState struct {
	ID          uint   // PK, incremental
	StateNumber uint   // incremental, source of truth for ordering (created at is just for data trailing), prevents collisions in case of concurrency
	SessionID   uint   // References same domain sessions.ID. UNIQUE INDEX(session_id, state_number)
	JSONState   string // JSON content of the state
	CreatedAt   time.Time
}
