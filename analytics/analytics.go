// Package analytics is Analytics' public contract: the event store the
// long-term metrics are computed from, fed by events the domains enqueue.
// This milestone is ingestion only. It imports only the standard library.
package analytics

import "time"

// Event types. Later milestones add the commercial ones.
const (
	EventGameCreated    = "game-created"
	EventGameReady      = "game-ready"
	EventGameEdited     = "game-edited"
	EventSessionStarted = "session-started"
)

// EventTypes lists every event type ingestion accepts.
var EventTypes = []string{EventGameCreated, EventGameReady, EventGameEdited, EventSessionStarted}

// ValidEventType reports whether t is a known event type.
func ValidEventType(t string) bool {
	for _, known := range EventTypes {
		if t == known {
			return true
		}
	}
	return false
}

// Account kinds and plan kinds, as they were when the event happened.
const (
	AccountPersonal = "personal"
	AccountTeacher  = "teacher"

	PlanFree         = "free"
	PlanTrial        = "trial"
	PlanSubscription = "subscription"
)

// ValidAccountKind reports whether k is a known account kind.
func ValidAccountKind(k string) bool {
	return k == AccountPersonal || k == AccountTeacher
}

// ValidPlanKind reports whether k is a known plan kind.
func ValidPlanKind(k string) bool {
	return k == PlanFree || k == PlanTrial || k == PlanSubscription
}

// Event is one analytics event from SourceDomain. UserRef is the creator the
// event belongs to, never a student; EntityRef is the game or session; the
// idempotency key is unique within the source domain.
type Event struct {
	EventType      string
	UserRef        string
	AccountKind    string
	PlanKind       string
	EntityRef      string
	Properties     map[string]any
	IdempotencyKey string
	OccurredAt     time.Time
}

// IngestResult counts what a batch did. Invalid events are skipped and
// counted; the valid ones in the same batch are still stored.
type IngestResult struct {
	Stored     int
	Duplicates int
	Invalid    int
}
