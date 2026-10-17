package main

import (
	"context"
	"errors"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

var errNotWired = errors.New("capability is not implemented yet")

// unwiredPlayerAdmission stands in for the Runtime's player admission.
//
// Placeholder: every call fails, so the API closes the connection as a server failure.
type unwiredPlayerAdmission struct{}

func (unwiredPlayerAdmission) AttachPlayer(context.Context, string, string, int64) (runtime.AttachPlayerResult, error) {
	return runtime.AttachPlayerResult{}, errNotWired
}
