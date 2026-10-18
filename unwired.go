package main

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/runtime/replay"
)

// unwiredReplay stands in for the replay recorder of running games.
//
// Placeholder: every capture is dropped; no replay history is recorded.
type unwiredReplay struct{}

func (unwiredReplay) Offer(context.Context, replay.Capture) {}
