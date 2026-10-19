package main

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/runtime/replay"
)

// unwiredReplay stands in for the replay recorder of running games.
//
// Placeholder: every capture is dropped, a finished game's final one too.
type unwiredReplay struct{}

func (unwiredReplay) Offer(context.Context, replay.Capture) {}

func (unwiredReplay) Finish(context.Context, replay.Capture) error { return nil }
