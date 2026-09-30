package sessionlifecycle

import (
	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
)

// collectOutboundEvents extracts every *platform.SendEvent already present
// in commands, in original order, and mirrors each one into a public
// session.OutboundEvent. It is a pure conversion, not a lookup: every
// platform.ActorRef a SendEvent carries was already validated by
// platform.ParseCommand against the caller's own known-actor set, so this
// function cannot fail and performs no database access.
func collectOutboundEvents(commands []platform.Command) []session.OutboundEvent {
	var events []session.OutboundEvent
	for _, c := range commands {
		sendEvent, ok := c.(*platform.SendEvent)
		if !ok {
			continue
		}
		recipients := make([]session.ActorRef, len(sendEvent.Recipients))
		for i, r := range sendEvent.Recipients {
			recipients[i] = session.ActorRef(r)
		}
		events = append(events, session.OutboundEvent{
			Recipients: recipients,
			Name:       sendEvent.Name,
			Payload:    sendEvent.Payload,
		})
	}
	return events
}
