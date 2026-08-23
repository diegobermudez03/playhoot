package sessionlifecycle

import (
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

type Manager struct {
	generalRepo    repo.GeneralPurposeAPI
	createRoomRepo repo.CreateRoomRepoAPI
	joinRoomRepo   repo.JoinRoomRepoAPI
}

func NewSessionLifecycleManager(db *gorm.DB) *Manager {
	repository := repo.NewSessionRepo(db)
	return &Manager{
		generalRepo:    repository,
		createRoomRepo: repository,
		joinRoomRepo:   repository,
	}
}
