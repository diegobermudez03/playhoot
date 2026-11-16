package game

import "time"

// QueueDomain names Game's queue: its table is game_queue_tasks.
const QueueDomain = "game"

// Analytics event constants Game emits. Until accounts (H3) and plans (H6)
// provide real values, every event carries the personal account kind and
// the free plan kind.
const (
	AnalyticsGameCreated   = "game-created"
	AnalyticsAccountKind   = "personal"
	AnalyticsPlanKind      = "free"
	analyticsKeyGamePrefix = "game-created:"
)

// AnalyticsEvent is the analytics-event task payload Game enqueues.
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

// GameCreatedEvent is the event of a Game created by ownerUUID at at.
func GameCreatedEvent(ownerUUID string, gameUUID string, at time.Time) AnalyticsEvent {
	return AnalyticsEvent{
		EventType: AnalyticsGameCreated, UserRef: ownerUUID, AccountKind: AnalyticsAccountKind, PlanKind: AnalyticsPlanKind,
		EntityRef: gameUUID, Properties: map[string]any{}, IdempotencyKey: analyticsKeyGamePrefix + gameUUID, OccurredAt: at,
	}
}
