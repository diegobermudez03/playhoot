// Snapshot persistence.
//
// engineservice deliberately does not expose a codec for engine.Program
// itself: a compiled Program is a pure, deterministic function of the
// program.Definition Compile was given (see Compile's own doc comment),
// and program.Definition is already fully encodable through
// gameservice.EncodeJSON/DecodeJSON. A session layer that needs to
// persist a game version should persist (or version-reference) the
// Definition, then call Compile again on load — recompiling is cheap
// and deterministic, and it avoids maintaining a second, redundant wire
// format for the same information the Definition's own codec already
// covers.
//
// A Snapshot has no such source-level counterpart — it is pure runtime
// state — so EncodeSnapshot and DecodeSnapshot below are what let a
// session layer actually persist and restore a game instance between
// process restarts.
package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/engine/internal/codec"
)

// DecodeError describes a structural failure while decoding the JSON
// wire representation of a Snapshot. See the underlying
// engine/internal/codec.DecodeError for field and path documentation.
type DecodeError = codec.DecodeError

// EncodeSnapshot encodes snapshot as compact JSON.
//
// EncodeSnapshot performs no compatibility validation against any
// particular engine.Program — call CheckSnapshotCompatibility separately
// if you need that, typically right after DecodeSnapshot when resuming
// a persisted game instance against a possibly newer compiled Program.
func EncodeSnapshot(snapshot engine.Snapshot) ([]byte, error) {
	return codec.EncodeSnapshot("$", snapshot)
}

// DecodeSnapshot decodes data as an engine.Snapshot.
//
// DecodeSnapshot requires data to hold exactly one JSON object; every
// nested value is decoded strictly, so a structural problem anywhere —
// an unrecognized discriminator, an unexpected field, truncated JSON —
// is reported as a path-aware *DecodeError rooted at "$". DecodeSnapshot
// is structurally strict but says nothing about whether the result is
// compatible with any particular engine.Program; call
// CheckSnapshotCompatibility separately for that.
func DecodeSnapshot(data []byte) (engine.Snapshot, error) {
	return codec.DecodeSnapshot("$", data)
}

// CheckSnapshotCompatibility reports whether snapshot can be resumed
// against p: its root instance must run p's own RootWorkflow, and every
// instance anywhere in its child-workflow tree (root, every occupied
// ChildSlot, and every task-group task) must run a workflow p actually
// compiles. This is the same root-workflow check Step itself makes
// (ExecutionErrorSnapshotProgramMismatch), exposed standalone so a
// caller restoring a persisted Snapshot can validate it once, before
// resuming — for example, after DecodeSnapshot, or after recompiling a
// program.Definition to a newer engine.Program version and before
// stepping an older persisted Snapshot against it.
func CheckSnapshotCompatibility(p engine.Program, snapshot engine.Snapshot) error {
	if snapshot.Root.Workflow != p.RootWorkflow {
		return &ExecutionError{Code: ExecutionErrorSnapshotProgramMismatch, Message: fmt.Sprintf(
			"engineservice: snapshot's root instance runs workflow %q, but this program's root workflow is %q", snapshot.Root.Workflow, p.RootWorkflow)}
	}
	return checkInstanceCompatibility(p, snapshot.Root)
}

func checkInstanceCompatibility(p engine.Program, instance engine.WorkflowInstance) error {
	if _, ok := p.Workflows[instance.Workflow]; !ok {
		return &ExecutionError{Code: ExecutionErrorSnapshotProgramMismatch, Message: fmt.Sprintf(
			"engineservice: snapshot references workflow %q, which this program does not compile", instance.Workflow)}
	}
	for _, s := range instance.ChildSlots {
		if s.Child != nil {
			if err := checkInstanceCompatibility(p, *s.Child); err != nil {
				return err
			}
		}
	}
	for _, s := range instance.TaskGroupSlots {
		if s.Group == nil {
			continue
		}
		for _, t := range s.Group.Tasks {
			if err := checkInstanceCompatibility(p, t.Child); err != nil {
				return err
			}
		}
	}
	return nil
}
