// Package gamecard is a Game's card in Game Management: its texts and the
// images it shows, read and changed by its owner, published or not. A
// Manager whose methods are the card steps, one file per step.
package gamecard

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamecard/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// imageStore keeps card image content and signs links to read it: the
// browser downloads a card image directly, the backend never reads it back.
type imageStore interface {
	Put(ctx context.Context, key string, content []byte, contentType string) error
	PresignGet(ctx context.Context, key string, expiresAt time.Time) (string, error)
}

// imagePrefix is where card image content is stored, content-addressed.
const imagePrefix = "games/card-images"

// imageLinkTTL is how long a card read's image links work.
const imageLinkTTL = 15 * time.Minute

// Manager is the card workflow controller. Each step depends on its own
// narrow persistence contract, all satisfied by one internal Repo.
type Manager struct {
	readCardRepo    readCardRepo
	updateCardRepo  updateCardRepo
	uploadImageRepo uploadImageRepo
	removeImageRepo removeImageRepo

	images imageStore

	// dbServicer is the handle utils.RunInDBTransaction opens transactions
	// on; it is the same Repo, which owns the database handle.
	dbServicer utils.DBServicer

	now     func() time.Time
	newUUID func() string
}

// New constructs a Manager persisting to db and keeping images in images.
func New(db *gorm.DB, images imageStore) *Manager {
	repo := internalrepo.New(db)
	return &Manager{
		readCardRepo:    repo,
		updateCardRepo:  repo,
		uploadImageRepo: repo,
		removeImageRepo: repo,
		images:          images,
		dbServicer:      repo,
		now:             time.Now,
		newUUID:         uuid.NewString,
	}
}

// isOwner reports whether recorded and user name the same user.
func isOwner(recorded string, user uuid.UUID) bool {
	owner, err := uuid.Parse(recorded)
	return err == nil && owner == user
}
