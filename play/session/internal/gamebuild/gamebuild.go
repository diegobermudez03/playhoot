// Package gamebuild is the Session domain's port onto the game toolchain that
// builds an SDK project into a version's backend script, frontend package
// and manifest. Nothing above it knows the toolchain runs as a Node process,
// so it can become a service of its own.
package gamebuild

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
)

// ErrUnavailable is returned when no toolchain is configured or it cannot
// start: the platform's failure, never the game's.
var ErrUnavailable = errors.New("game toolchain unavailable")

// Input is a project to build: a directory holding its sources.
type Input struct {
	Dir string
}

// Output is what building a project produced. OK means the artifacts are
// set; otherwise Diagnostics say what the author must fix.
type Output struct {
	OK          bool
	Server      []byte
	Client      []byte
	Manifest    json.RawMessage
	AspectRatio string
	// What the client asks of the device, as the toolchain read it from the sources.
	InputRequirements InputRequirements
	Diagnostics       []build.Diagnostic
}

// InputRequirements is what a built client needs: a physical keyboard, hovering, pressing.
type InputRequirements struct {
	UsesKeyboard     bool
	UsesPointerHover bool
	UsesPointerPress bool
}

// Builder builds SDK projects. An error is the toolchain itself failing
// (ErrUnavailable, a timeout, a crash), never a problem of the game.
type Builder interface {
	// Version identifies the toolchain: the same sources built by the same
	// version give the same artifacts.
	Version(ctx context.Context) (string, error)
	Build(ctx context.Context, input Input) (Output, error)
}

// Unconfigured is the Builder when no toolchain is configured: every build
// fails as the platform's.
type Unconfigured struct{}

// Version implements Builder.
func (Unconfigured) Version(ctx context.Context) (string, error) {
	return "", ErrUnavailable
}

// Build implements Builder.
func (Unconfigured) Build(ctx context.Context, input Input) (Output, error) {
	return Output{}, ErrUnavailable
}
