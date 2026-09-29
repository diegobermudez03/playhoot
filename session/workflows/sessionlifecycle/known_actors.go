package sessionlifecycle

import (
	"strconv"

	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
)

// actorRefForActorID renders an internal session_actors.id as the opaque
// decimal-string platform.ActorRef every Event/Command carries - the same
// representation step_start.go's engine.UserValue construction used to use,
// now the platform's own boundary type instead of an engine-specific one.
func actorRefForActorID(actorID uint) platform.ActorRef {
	return platform.ActorRef(strconv.FormatUint(uint64(actorID), 10))
}

// knownActorsFromRoster converts roster (an already-loaded active-
// Participant roster, ListActiveParticipantsForRoster's own return shape)
// into the platform.KnownActors set ParseEvent/ParseCommand validate every
// actor reference against - every RUNNING-phase step's own Event/Command
// boundary needs exactly this same set, since a Session's addressable
// actors are always its currently active Participants.
func knownActorsFromRoster(roster []internalrepo.RosterParticipant) platform.KnownActors {
	refs := make([]platform.ActorRef, len(roster))
	for i, p := range roster {
		refs[i] = actorRefForActorID(p.ActorID)
	}
	return platform.NewKnownActors(refs...)
}
