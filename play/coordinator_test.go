package play

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// fakeConn is a Conn double recording every Event it receives, optionally
// failing every Deliver call to prove a failed delivery never surfaces
// back to the caller of Coordinator.Deliver.
type fakeConn struct {
	fail     bool
	received []Event
}

func (c *fakeConn) Deliver(e Event) error {
	c.received = append(c.received, e)
	if c.fail {
		return errors.New("delivery failed")
	}
	return nil
}

func TestCoordinator_CreateJoin_ForwardDirectlyToSessionRuntime(t *testing.T) {
	ctrl := gomock.NewController(t)
	rt := NewMockSessionRuntime(ctrl)
	c := NewCoordinator(rt)
	ctx := context.Background()

	rt.EXPECT().Create(ctx, "game-1", "host-1", "idem-1").Return(CreatedSession{SessionUUID: "session-1", JoinCode: 1234}, nil)
	created, err := c.Create(ctx, "game-1", "host-1", "idem-1")
	require.NoError(t, err)
	require.Equal(t, CreatedSession{SessionUUID: "session-1", JoinCode: 1234}, created)

	rt.EXPECT().Join(ctx, uint(1234), "user-1", "Alice", "idem-2").Return(JoinResult{Outcome: JoinOutcomeJoined, SessionUUID: "session-1"}, nil)
	joined, err := c.Join(ctx, 1234, "user-1", "Alice", "idem-2")
	require.NoError(t, err)
	require.Equal(t, JoinOutcomeJoined, joined.Outcome)
}

func TestCoordinator_Start_ReturnsEventsWithoutDeliveringThem(t *testing.T) {
	ctrl := gomock.NewController(t)
	rt := NewMockSessionRuntime(ctrl)
	c := NewCoordinator(rt)
	ctx := context.Background()

	bound := &fakeConn{}
	unbind := c.Bind("session-1", "user-bound", bound)
	defer unbind()

	events := []Event{
		{Recipient: "user-bound", Kind: EventKindInteractionOpened, InteractionID: "interaction-1", Question: "Pick", Arguments: map[string]any{"min": float64(1)}},
	}
	rt.EXPECT().Start(ctx, "session-1", "user-bound", "idem-1").Return(StartResult{Outcome: StartOutcomeStarted, Events: events}, nil)

	outcome, gotEvents, err := c.Start(ctx, "session-1", "user-bound", "idem-1")
	require.NoError(t, err)
	require.Equal(t, StartOutcomeStarted, outcome)
	require.Equal(t, events, gotEvents)

	// Start itself never delivers - a caller must call Deliver explicitly,
	// so it can sequence its own direct reply first.
	require.Empty(t, bound.received)
}

func TestCoordinator_Start_NoEventsReturnedWhenSessionRuntimeErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	rt := NewMockSessionRuntime(ctrl)
	c := NewCoordinator(rt)
	ctx := context.Background()

	rt.EXPECT().Start(ctx, "session-1", "user-bound", "idem-1").Return(StartResult{}, errors.New("boom"))

	_, events, err := c.Start(ctx, "session-1", "user-bound", "idem-1")
	require.Error(t, err)
	require.Empty(t, events)
}

func TestCoordinator_AnswerInteraction_ReturnsEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	rt := NewMockSessionRuntime(ctrl)
	c := NewCoordinator(rt)
	ctx := context.Background()

	events := []Event{{Recipient: "user-1", Kind: EventKindInteractionClosed, InteractionID: "interaction-1"}}
	rt.EXPECT().AnswerInteraction(ctx, "interaction-1", "user-1", []byte("42")).Return(AnswerInteractionResult{Outcome: AnswerOutcomeAnswered, Events: events}, nil)

	outcome, gotEvents, err := c.AnswerInteraction(ctx, "session-1", "interaction-1", "user-1", []byte("42"))
	require.NoError(t, err)
	require.Equal(t, AnswerOutcomeAnswered, outcome)
	require.Equal(t, events, gotEvents)
}

func TestCoordinator_Deliver_OnlyReachesBoundRecipientsAndSurvivesFailure(t *testing.T) {
	c := NewCoordinator(nil)

	bound := &fakeConn{}
	unbind := c.Bind("session-1", "user-bound", bound)
	defer unbind()
	failing := &fakeConn{fail: true}
	unbindFailing := c.Bind("session-1", "user-failing", failing)
	defer unbindFailing()

	events := []Event{
		{Recipient: "user-bound", Kind: EventKindInteractionOpened, InteractionID: "interaction-1"},
		{Recipient: "user-not-bound", Kind: EventKindInteractionOpened, InteractionID: "interaction-2"},
		{Recipient: "user-failing", Kind: EventKindInteractionClosed, InteractionID: "interaction-3"},
	}

	// Must not panic or otherwise surface failing's Deliver error.
	c.Deliver("session-1", events)

	require.Len(t, bound.received, 1)
	require.Equal(t, events[0], bound.received[0])
	require.Len(t, failing.received, 1)
	require.Equal(t, events[2], failing.received[0])
}

func TestCoordinator_Bind_UnbindStopsFurtherDelivery(t *testing.T) {
	c := NewCoordinator(nil)

	conn := &fakeConn{}
	unbind := c.Bind("session-1", "user-1", conn)
	unbind()

	events := []Event{{Recipient: "user-1", Kind: EventKindInteractionClosed, InteractionID: "interaction-1"}}
	c.Deliver("session-1", events)

	require.Empty(t, conn.received)
}

func TestCoordinator_Bind_ReplacesPriorConnectionForSameKey(t *testing.T) {
	c := NewCoordinator(nil)

	first := &fakeConn{}
	firstUnbind := c.Bind("session-1", "user-1", first)
	second := &fakeConn{}
	c.Bind("session-1", "user-1", second)

	// The first connection's own unbind, called after it was replaced by
	// second (as a stale reconnect's cleanup would), must not remove
	// second's registration.
	firstUnbind()

	events := []Event{{Recipient: "user-1", Kind: EventKindInteractionClosed, InteractionID: "interaction-1"}}
	c.Deliver("session-1", events)

	require.Empty(t, first.received)
	require.Len(t, second.received, 1)
}
