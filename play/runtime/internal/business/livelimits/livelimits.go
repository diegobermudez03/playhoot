// Package livelimits holds the platform's limits that shape what a running
// game observes: how many inputs, commands and boundaries reach it. The live
// Session controller and the simulation share them, so a simulated game is
// refused exactly where a played one would be.
package livelimits

import (
	"time"

	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// Limit names, as refusals and metrics name them.
const (
	SampleRate          = "sample_rate"
	TimelineParticipant = "timeline_participant"
	TimelineSession     = "timeline_session"
	CommandPending      = "command_pending"
	CommandParticipant  = "command_participant_rate"
	CommandSession      = "command_session_rate"
)

// SampleTransitions bound one participant's distinct input transitions.
func SampleTransitions() []ratelimit.Limit {
	return []ratelimit.Limit{{Count: 120, Per: time.Second}, {Count: 3600, Per: time.Minute}}
}

// ParticipantCommands bound one participant's COMMANDs.
func ParticipantCommands() []ratelimit.Limit {
	return []ratelimit.Limit{{Count: 10, Per: time.Second}, {Count: 120, Per: time.Minute}}
}

// SessionCommands bound every participant's COMMANDs together.
func SessionCommands() []ratelimit.Limit {
	return []ratelimit.Limit{{Count: 60, Per: time.Second}, {Count: 600, Per: time.Minute}}
}

// Boundaries bound how often a Session runs a boundary; ticks always run,
// so one asked for beyond them waits for the next tick. Ticks alone run 10
// a second, which leaves room for the boundaries COMMANDs ask for.
func Boundaries() []ratelimit.Limit {
	return []ratelimit.Limit{{Count: 25, Per: time.Second}, {Count: 1200, Per: time.Minute}}
}

const (
	// PendingCommands is how many of one participant's COMMANDs may wait at
	// once.
	PendingCommands = 4

	// PendingPerParticipant and PendingPerSession bound the players'
	// operations waiting in the timeline.
	PendingPerParticipant = 128
	PendingPerSession     = 2048

	// BoundaryOperations is the most timeline operations one boundary
	// executes; the rest wait for the next.
	BoundaryOperations = 256

	// TickInterval is a running game's boundary cadence.
	TickInterval = 100 * time.Millisecond
)
