package service

import (
	"github.com/diegobermudez03/playhoot/game/session/internal/repo"
	"gorm.io/gorm"
)

type SessionManager struct {
	repo repo.SessionRepoAPI
}

func NewSessionManager(db *gorm.DB) *SessionManager {
	return &SessionManager{
		repo: repo.NewSessionRepo(db),
	}
}
