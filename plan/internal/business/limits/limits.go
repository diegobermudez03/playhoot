// Package limits holds the pure rules of the free limits: the defaults and
// what a valid configuration is.
package limits

import "github.com/diegobermudez03/playhoot/plan"

// Bounds of a valid configuration.
const (
	MaxCount      = 100000
	MaxWindowDays = 365
	MaxStorageMB  = 1000000
	MaxReasonLen  = 500
)

// Defaults are the free limits when no configuration is in force. They match
// the seeded configuration.
var Defaults = plan.Limits{
	SavedGames:         3,
	CreationsPerWindow: 2,
	SessionsPerWindow:  5,
	WindowDays:         30,
	PlayersPerSession:  10,
	SimultaneousRooms:  1,
	StorageMB:          100,
}

// Valid reports whether l can be configured. Zero is a valid capacity (it
// only stops adding more); the window needs at least a day.
func Valid(l plan.Limits) bool {
	counts := []int{l.SavedGames, l.CreationsPerWindow, l.SessionsPerWindow, l.PlayersPerSession, l.SimultaneousRooms}
	for _, count := range counts {
		if count < 0 || count > MaxCount {
			return false
		}
	}
	return l.WindowDays >= 1 && l.WindowDays <= MaxWindowDays && l.StorageMB >= 0 && l.StorageMB <= MaxStorageMB
}
