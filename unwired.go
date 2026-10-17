package main

import (
	"context"
	"errors"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

var errNotWired = errors.New("capability is not implemented yet")

// unwiredPlayerAdmission stands in for the Runtime's player admission.
//
// Placeholder: every call fails, so the API closes the connection as a server failure.
type unwiredPlayerAdmission struct{}

func (unwiredPlayerAdmission) AttachPlayer(context.Context, string, string, int64) (runtime.AttachPlayerResult, error) {
	return runtime.AttachPlayerResult{}, errNotWired
}

// unwiredJoinCodes stands in for the durable Session's join code lookup.
//
// Placeholder: every call fails, so joining by join code answers 500.
type unwiredJoinCodes struct{}

func (unwiredJoinCodes) ResolveJoinCode(context.Context, string) (sessionpkg.JoinCodeResolution, error) {
	return sessionpkg.JoinCodeResolution{}, errNotWired
}
