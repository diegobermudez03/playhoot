// Package liveconnection is the workflow of a client's live connection to a
// Session, from admission until it ends: a Manager whose methods are the
// workflow's operations, one file per operation.
package liveconnection

import (
	"context"
	"crypto/rand"
	"encoding/binary"

	"github.com/diegobermudez03/playhoot/play/runtime/internal/controller"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=manager_mock_test.go . manifestLoader

// manifestLoader reads the manifest of the game version a Session is pinned
// to, which a new controller keeps for the Session's lifetime.
type manifestLoader interface {
	LoadRuntimeManifest(ctx context.Context, sessionUUID string) (sessionpkg.RuntimeManifest, error)
}

// Manager is the live connection workflow controller.
type Manager struct {
	adminJoins  adminJoinChecker
	playerJoins playerJoinChecker
	manifests   manifestLoader
	starts      sessionStarter
	frontends   frontendSigner
	sandbox     gameSandbox
	controllers *controller.Registry
	// seeds returns the seed of a starting game's randomness.
	seeds func() uint64
}

// New constructs a Manager. adminJoins and playerJoins decide who may join a
// Session; manifests describe the game a Session plays; starts records a
// Session's start, frontends grant its game's frontend and sandbox runs it.
func New(adminJoins adminJoinChecker, playerJoins playerJoinChecker, manifests manifestLoader, starts sessionStarter, frontends frontendSigner, sandbox gameSandbox) *Manager {
	return &Manager{
		adminJoins:  adminJoins,
		playerJoins: playerJoins,
		manifests:   manifests,
		starts:      starts,
		frontends:   frontends,
		sandbox:     sandbox,
		controllers: controller.NewRegistry(),
		seeds:       randomSeed,
	}
}

func randomSeed() uint64 {
	var seed [8]byte
	// crypto/rand.Read never fails.
	_, _ = rand.Read(seed[:])
	return binary.LittleEndian.Uint64(seed[:])
}

// controllerFor finds or creates sessionUUID's controller under ownerEpoch,
// loading the Session's manifest when it is created.
func (m *Manager) controllerFor(ctx context.Context, sessionUUID string, ownerEpoch int64) (*controller.Controller, bool, error) {
	return m.controllers.ControllerFor(ctx, sessionUUID, ownerEpoch, func(ctx context.Context) (controller.Manifest, error) {
		manifest, err := m.manifests.LoadRuntimeManifest(ctx, sessionUUID)
		if err != nil {
			return controller.Manifest{}, err
		}
		return controller.Manifest{MinPlayers: manifest.MinPlayers, MaxPlayers: manifest.MaxPlayers}, nil
	})
}
