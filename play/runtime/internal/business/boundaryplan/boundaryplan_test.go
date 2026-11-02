package boundaryplan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
)

func transition(participant string, atMs int64, order uint64) Operation {
	return Operation{ParticipantID: participant, SessionTimeMs: atMs, Order: order, Inputs: []byte(`{"x":1}`)}
}

func command(participant string, atMs int64, order uint64) Operation {
	return Operation{ParticipantID: participant, SessionTimeMs: atMs, Order: order, Command: &Command{Name: "go", Payload: []byte(`{}`)}}
}

func TestPlan(t *testing.T) {
	tests := map[string]struct {
		pending  []Operation
		now      int64
		through  int64
		update   bool
		limit    int
		expected Boundary
	}{
		"an empty tick runs update at its time": {now: 500, through: 400, update: true, expected: Boundary{SessionTimeMs: 500, Update: true}},
		"never before the confirmed state":      {now: 300, through: 400, update: true, expected: Boundary{SessionTimeMs: 400, Update: true}},
		"a command flush with update ends at the boundary": {
			pending: []Operation{transition("P2", 120, 1), command("P1", 150, 2)}, now: 150, update: true,
			expected: Boundary{Taken: 2, SessionTimeMs: 150, Update: true},
		},
		"without update a command ends the boundary at itself": {
			pending: []Operation{command("P1", 150, 1)}, now: 160,
			expected: Boundary{Taken: 1, SessionTimeMs: 150},
		},
		"the operation limit splits without update": {
			pending: []Operation{transition("P1", 10, 1), transition("P1", 20, 2), transition("P1", 30, 3)}, now: 100, update: true, limit: 2,
			expected: Boundary{Taken: 2, SessionTimeMs: 20},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, Plan(tc.pending, tc.now, tc.through, tc.update, tc.limit))
		})
	}
}

func TestBatch(t *testing.T) {
	taken := []Operation{transition("P1", 110, 1), transition("P2", 120, 2), command("P1", 150, 3), transition("P2", 160, 4)}
	batch, orders := Batch(taken, Boundary{Taken: 4, SessionTimeMs: 200, Update: true}, 100)

	require.Len(t, batch.Operations, 4)
	require.Len(t, batch.Operations[0].Transitions, 2, "adjacent transitions share a samples() call")
	require.Equal(t, int64(120), batch.Operations[0].SessionTimeMs, "a run of transitions followed by more runs until its last one")
	require.Equal(t, &sandbox.Command{ParticipantID: "P1", Name: "go", Payload: []byte(`{}`)}, batch.Operations[1].Command)
	require.Equal(t, int64(150), batch.Operations[1].SessionTimeMs)
	require.Equal(t, int64(160), batch.Operations[2].SessionTimeMs)
	require.True(t, batch.Operations[3].Update)
	require.Equal(t, int64(200), batch.Operations[3].SessionTimeMs)
	require.Equal(t, []uint64{2, 3, 4, 0}, orders)

	t.Run("a run of transitions that ends a batch without update runs until the boundary", func(t *testing.T) {
		batch, _ := Batch([]Operation{transition("P1", 110, 1)}, Boundary{Taken: 1, SessionTimeMs: 200}, 100)
		require.Equal(t, int64(200), batch.Operations[0].SessionTimeMs)
	})
}

func TestFinishedAt(t *testing.T) {
	taken := []Operation{transition("P1", 110, 1), command("P1", 150, 2), transition("P2", 160, 3)}
	batch, orders := Batch(taken, Boundary{Taken: 3, SessionTimeMs: 200, Update: true}, 100)

	executed, at, ok := FinishedAt(taken, batch, orders, 1)
	require.True(t, ok)
	require.Equal(t, 2, executed, "what follows the operation that finished never happened")
	require.Equal(t, int64(150), at)

	executed, at, ok = FinishedAt(taken, batch, orders, len(batch.Operations)-1)
	require.True(t, ok)
	require.Equal(t, 3, executed, "update finishing ran after every operation")
	require.Equal(t, int64(200), at)

	_, _, ok = FinishedAt(taken, batch, orders, 9)
	require.False(t, ok)
}
