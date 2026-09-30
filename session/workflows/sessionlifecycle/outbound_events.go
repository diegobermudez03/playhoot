package sessionlifecycle

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
)

// collectOutboundEvents extracts every *platform.SendEvent already present
// in commands, in original order, and mirrors each one into a public
// session.OutboundEvent. It is a pure conversion, not a lookup: every
// platform.ActorRef a SendEvent carries was already validated by
// platform.ParseCommand against the caller's own known-actor set, so this
// function cannot fail and performs no database access.
//
// sequence is the RuntimeTurn this call's own execute pass committed (or
// replayed) commands under - the same value already passed to
// CreateRuntimeTurn at each call site. It becomes each event's own Revision,
// and (together with the event's own position in commands) its own ID: a
// stable, deterministic identifier reproducible for the exact same
// underlying command across a delivery retry, never randomly regenerated.
func collectOutboundEvents(sequence uint64, commands []platform.Command) []session.OutboundEvent {
	var events []session.OutboundEvent
	for i, c := range commands {
		sendEvent, ok := c.(*platform.SendEvent)
		if !ok {
			continue
		}
		recipients := make([]session.ActorRef, len(sendEvent.Recipients))
		for j, r := range sendEvent.Recipients {
			recipients[j] = session.ActorRef(r)
		}
		events = append(events, session.OutboundEvent{
			ID:         fmt.Sprintf("%d:%d", sequence, i),
			Recipients: recipients,
			Name:       sendEvent.Name,
			Payload:    sendEvent.Payload,
			Revision:   sequence,
		})
	}
	return events
}
