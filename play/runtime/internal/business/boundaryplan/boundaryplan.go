// Package boundaryplan decides what one boundary of a running game executes:
// which of the operations waiting in the timeline it takes, the Session time
// it brings the game to, and the sandbox batch that runs them. The live
// Session controller and the simulation share it, so a simulated game runs
// exactly as a played one.
package boundaryplan

import "github.com/diegobermudez03/playhoot/play/runtime/sandbox"

// Operation is one operation waiting in the timeline: an input transition
// (Inputs set), a command, a disconnection or a rejoin, at the Session time
// it was accepted and in acceptance order.
type Operation struct {
	ParticipantID string
	SessionTimeMs int64
	Order         uint64

	// Inputs is the full, normalized input snapshot of a transition.
	Inputs []byte

	Command      *Command
	Disconnected bool
	Rejoin       bool
}

// Command is a command operation's validated name and full payload.
type Command struct {
	Name    string
	Payload []byte
}

// RunsAlone reports whether the operation is its own call, at its own time,
// rather than a transition sharing a samples() call.
func (o Operation) RunsAlone() bool {
	return o.Command != nil || o.Disconnected || o.Rejoin
}

// Boundary is what a boundary takes: the first Taken operations of the
// timeline, the Session time it brings the game to, and whether it ends with
// update().
type Boundary struct {
	Taken         int
	SessionTimeMs int64
	Update        bool
}

// Plan decides the boundary run at nowMs over pending, for a game confirmed
// through executedThroughMs that declares update() when update is true. At
// most limit operations are taken (0: all); a boundary that leaves some ends
// at its last one, without update(), and so does one ending with an operation
// that runs alone when the game has no update().
func Plan(pending []Operation, nowMs int64, executedThroughMs int64, update bool, limit int) Boundary {
	planned := Boundary{Taken: len(pending), SessionTimeMs: max(nowMs, executedThroughMs)}
	split := limit > 0 && len(pending) > limit
	if split {
		planned.Taken = limit
	}
	planned.Update = update && !split
	if planned.Taken > 0 {
		last := pending[planned.Taken-1]
		planned.SessionTimeMs = max(planned.SessionTimeMs, last.SessionTimeMs)
		if (last.RunsAlone() && !planned.Update) || split {
			planned.SessionTimeMs = max(last.SessionTimeMs, executedThroughMs)
		}
	}
	return planned
}

// Batch turns a boundary's taken operations into one sandbox batch, with the
// order of the last timeline operation each call runs (0 for update).
// Adjacent transitions share a samples() call, each command, disconnection
// or rejoin runs alone, update() ends it; each runs at its own time, never
// before the confirmed state's.
func Batch(taken []Operation, planned Boundary, executedThroughMs int64) (sandbox.Batch, []uint64) {
	var batch sandbox.Batch
	var orders []uint64
	for _, operation := range taken {
		if operation.RunsAlone() {
			alone := sandbox.Operation{SessionTimeMs: operation.SessionTimeMs}
			switch {
			case operation.Command != nil:
				alone.Command = &sandbox.Command{ParticipantID: operation.ParticipantID, Name: operation.Command.Name, Payload: operation.Command.Payload}
			case operation.Rejoin:
				alone.Rejoined = operation.ParticipantID
			default:
				alone.Disconnected = operation.ParticipantID
			}
			batch.Operations = append(batch.Operations, alone)
			orders = append(orders, operation.Order)
			continue
		}
		transition := sandbox.Transition{ParticipantID: operation.ParticipantID, Inputs: operation.Inputs, SessionTimeMs: operation.SessionTimeMs}
		if n := len(batch.Operations); n > 0 && len(batch.Operations[n-1].Transitions) > 0 {
			batch.Operations[n-1].Transitions = append(batch.Operations[n-1].Transitions, transition)
			orders[n-1] = operation.Order
			continue
		}
		batch.Operations = append(batch.Operations, sandbox.Operation{Transitions: []sandbox.Transition{transition}})
		orders = append(orders, operation.Order)
	}
	if planned.Update {
		batch.Operations = append(batch.Operations, sandbox.Operation{SessionTimeMs: planned.SessionTimeMs, Update: true})
		orders = append(orders, 0)
	}
	at := executedThroughMs
	for i := range batch.Operations {
		operation := &batch.Operations[i]
		switch {
		case operation.Command != nil, operation.Update, operation.Disconnected != "", operation.Rejoined != "":
		case i+1 < len(batch.Operations):
			operation.SessionTimeMs = operation.Transitions[len(operation.Transitions)-1].SessionTimeMs
		default:
			operation.SessionTimeMs = planned.SessionTimeMs
		}
		operation.SessionTimeMs = max(operation.SessionTimeMs, at)
		at = operation.SessionTimeMs
	}
	return batch, orders
}

// FinishedAt is where the batch operation at index finished the game: how
// many of the taken operations it executed and the Session time it ended at.
// ok is false when the batch has no operation at index, which means the
// context drifted.
func FinishedAt(taken []Operation, batch sandbox.Batch, orders []uint64, index int) (executed int, sessionTimeMs int64, ok bool) {
	if index < 0 || index >= len(batch.Operations) {
		return 0, 0, false
	}
	executed = len(taken)
	// update() runs after every timeline operation; its order is 0.
	if through := orders[index]; through != 0 {
		executed = 0
		for executed < len(taken) && taken[executed].Order <= through {
			executed++
		}
	}
	return executed, batch.Operations[index].SessionTimeMs, true
}
