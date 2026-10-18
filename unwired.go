package main

import (
	"context"
	"errors"

	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

var errNotWired = errors.New("capability is not implemented yet")

// unwiredSessionStart stands in for the durable Session's side of starting a game.
//
// Placeholder: every call fails, so starting answers the admin as a server failure.
type unwiredSessionStart struct{}

func (unwiredSessionStart) CheckStartEligibility(context.Context, string, string, int) (sessionpkg.StartEligibility, error) {
	return sessionpkg.StartEligibility{}, errNotWired
}

func (unwiredSessionStart) FrontendAccess(context.Context, string) (sessionpkg.FrontendAccess, error) {
	return sessionpkg.FrontendAccess{}, errNotWired
}

func (unwiredSessionStart) StoreCheckpoint(context.Context, string, []byte) (string, error) {
	return "", errNotWired
}

func (unwiredSessionStart) CommitStart(context.Context, string, string, sessionpkg.StartCommit) (sessionpkg.CommitStartOutcome, error) {
	return "", errNotWired
}

func (unwiredSessionStart) AbortStart(context.Context, string) error {
	return errNotWired
}

// unwiredSandbox stands in for the sandbox's execution contexts.
//
// Placeholder: every call fails.
type unwiredSandbox struct{}

func (unwiredSandbox) CreateContext(context.Context, sandbox.ContextSpec) (string, error) {
	return "", errNotWired
}

func (unwiredSandbox) Start(context.Context, string, sandbox.StartInput) (sandbox.StartResult, error) {
	return sandbox.StartResult{}, errNotWired
}

func (unwiredSandbox) Project(context.Context, string, string) (sandbox.ProjectResult, error) {
	return sandbox.ProjectResult{}, errNotWired
}

func (unwiredSandbox) DiscardContext(context.Context, string) error {
	return errNotWired
}
