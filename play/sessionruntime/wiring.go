package sessionruntime

import (
	"github.com/diegobermudez03/playhoot/game/management/usecases/getgame"
	"github.com/diegobermudez03/playhoot/game/management/usecases/getgamedefinition"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle"
	"github.com/diegobermudez03/playhoot/play"
	"gorm.io/gorm"
)

// NewCoordinator builds a fully wired, in-process play.Coordinator over db:
// a real sessionlifecycle.Manager backed by db's own current/pinned Game
// Definition readers, bridged through this package's SessionRuntime
// implementation. This is the one constructor a caller that only has a
// database handle needs - it is expected to be the sole place that knows
// how to assemble a working Coordinator from scratch, so replacing this
// in-process assembly with a network client later touches only this
// function, not its callers.
func NewCoordinator(db *gorm.DB) *play.Coordinator {
	manager := sessionlifecycle.New(db, getgame.New(db), getgamedefinition.New(db))
	return play.NewCoordinator(New(manager, db))
}
