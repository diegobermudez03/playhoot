package repo

import "context"

type CreateRoomRepoAPI interface{}

func (r *Repo) CreateRoom(ctx context.Context)
