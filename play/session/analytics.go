package session

import "time"

// QueueDomain names Session's queue: its table is play_session_queue_tasks.
const QueueDomain = "play_session"

// Analytics event constants Session emits. Until accounts (H3) and plans
// (H6) provide real values, every event carries the personal account kind
// and the free plan kind.
const (
	AnalyticsSessionStarted = "session-started"
	AnalyticsAccountKind    = "personal"
	AnalyticsPlanKind       = "free"
)

// AnalyticsEvent is the analytics-event task payload Session enqueues.
type AnalyticsEvent struct {
	EventType      string         `json:"event_type"`
	UserRef        string         `json:"user_ref"`
	AccountKind    string         `json:"account_kind"`
	PlanKind       string         `json:"plan_kind"`
	EntityRef      string         `json:"entity_ref"`
	Properties     map[string]any `json:"properties"`
	IdempotencyKey string         `json:"idempotency_key"`
	OccurredAt     time.Time      `json:"occurred_at"`
}

// SessionStartedEvent is the event of sessionUUID started by creatorUUID at
// at. real_participants is true when someone other than the creator joined.
func SessionStartedEvent(creatorUUID string, sessionUUID string, realParticipants bool, at time.Time) AnalyticsEvent {
	return AnalyticsEvent{
		EventType: AnalyticsSessionStarted, UserRef: creatorUUID, AccountKind: AnalyticsAccountKind, PlanKind: AnalyticsPlanKind,
		EntityRef: sessionUUID, Properties: map[string]any{"real_participants": realParticipants},
		IdempotencyKey: "session-started:" + sessionUUID, OccurredAt: at,
	}
}
