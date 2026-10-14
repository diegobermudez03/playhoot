package main

import (
	"context"
	"errors"

	"github.com/diegobermudez03/playhoot/identity"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
)

var errNotWired = errors.New("capability is not implemented yet")

// unwiredSessionCreator stands in for Orchestrator's CreateSession until it
// exists. Every call fails.
type unwiredSessionCreator struct{}

func (unwiredSessionCreator) CreateSession(context.Context, identity.UserUUID, sessionpkg.GameUUID, sessionpkg.IdempotencyKey) (sessionpkg.CreateSessionResult, error) {
	return sessionpkg.CreateSessionResult{}, errNotWired
}
