// Package plan is Plan's public contract: what each account is entitled to.
// For now that is the configurable free capacities every account gets; the
// catalog, purchases and credits arrive later. It imports only the standard
// library.
package plan

import "time"

// Limits are the capacities an account has: saved games (drafts included),
// creations and sessions started per rolling window of WindowDays, players
// per session, simultaneous rooms (lobbies included) and storage in MB. The
// owning domain applies the stricter of these and its own.
type Limits struct {
	SavedGames         int
	CreationsPerWindow int
	SessionsPerWindow  int
	WindowDays         int
	PlayersPerSession  int
	SimultaneousRooms  int
	StorageMB          int
}

// FreeLimitConfig is one entry of the free-limit history: Limits from
// EffectiveAt, who set them and why. CreatedByUserUUID is empty for the
// initial seeded entry.
type FreeLimitConfig struct {
	Limits            Limits
	EffectiveAt       time.Time
	CreatedByUserUUID string
	Reason            string
	CreatedAt         time.Time
}

// FreeLimitConfigs is the history as admin reads it: the entry in force, the
// entries scheduled to take effect, and the superseded ones, newest first.
type FreeLimitConfigs struct {
	Current   FreeLimitConfig
	Scheduled []FreeLimitConfig
	History   []FreeLimitConfig
}

// SetFreeLimitsOutcome is what AdminSetFreeLimits decided.
type SetFreeLimitsOutcome string

const (
	// FreeLimitsRecorded means the entry is appended.
	FreeLimitsRecorded SetFreeLimitsOutcome = "recorded"
	// FreeLimitsInvalid means a value is out of range, the reason is missing
	// or the effective date is in the past.
	FreeLimitsInvalid SetFreeLimitsOutcome = "invalid"
)
