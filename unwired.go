package main

import (
	"context"
	"errors"

	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
)

var errNotWired = errors.New("capability is not implemented yet")

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
