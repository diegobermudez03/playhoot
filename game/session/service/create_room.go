package service

import (
	"context"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/session"
)

func (m *SessionManager) CreateRoom(ctx context.Context, program engine.Program, gameVersionUUID string, ownerUUID string) (session.Room, error) {
	return session.Room{}, nil
}
