package main

import (
	"context"
	"errors"

	sessionpkg "github.com/diegobermudez03/playhoot/play"
)

var errNotWired = errors.New("capability is not implemented yet")

// unwiredSessionCreation stands in for the Session domain's creation until
// it exists. Every call fails.
type unwiredSessionCreation struct{}

func (unwiredSessionCreation) CreateSession(context.Context, string, string, string) (sessionpkg.CreateSessionResult, error) {
	return sessionpkg.CreateSessionResult{}, errNotWired
}
