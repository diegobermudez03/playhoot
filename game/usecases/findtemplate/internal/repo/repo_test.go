package repo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
	"github.com/diegobermudez03/playhoot/game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

type seededGame struct {
	ID               uint
	UUID             string
	Name             string
	Description      string
	OwnerUUID        string
	Visibility       string
	PublishedAt      *time.Time
	DeletedAt        *time.Time
	TemplateAllowed  bool
	TemplateFeatured bool
}

func (seededGame) TableName() string { return "games" }

type seededEdition struct {
	ID        uint
	UUID      string
	GameID    uint
	Status    string
	CreatedAt time.Time
}

func (seededEdition) TableName() string { return "game_editions" }

func seed(t *testing.T, db *gorm.DB, published bool, allowed bool, deleted bool) (string, string) {
	t.Helper()
	now := time.Now().UTC()
	row := seededGame{UUID: uuid.NewString(), Name: "Quiz", Description: "A quiz", OwnerUUID: uuid.NewString(), Visibility: "public", TemplateAllowed: allowed, TemplateFeatured: true}
	if published {
		row.PublishedAt = &now
	}
	if deleted {
		row.DeletedAt = &now
	}
	require.NoError(t, db.Create(&row).Error)
	first := seededEdition{UUID: uuid.NewString(), GameID: row.ID, Status: string(edition.Published), CreatedAt: now}
	latest := seededEdition{UUID: uuid.NewString(), GameID: row.ID, Status: string(edition.Published), CreatedAt: now}
	open := seededEdition{UUID: uuid.NewString(), GameID: row.ID, Status: string(edition.Editing), CreatedAt: now}
	for _, e := range []*seededEdition{&first, &latest, &open} {
		require.NoError(t, db.Create(e).Error)
	}
	return row.UUID, latest.UUID
}

func TestFindTemplate(t *testing.T) {
	db := testdb.OpenGameDB(t)
	repo := New(db)
	ctx := context.Background()

	gameUUID, latest := seed(t, db, true, true, false)
	found, err := repo.FindTemplate(ctx, gameUUID, string(edition.Published))
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, latest, *found.VersionUUID, "the latest published edition, never the open one")
	require.True(t, found.Featured)

	for name, flags := range map[string][3]bool{"not allowed": {true, false, false}, "unpublished": {false, true, false}, "deleted": {true, true, true}} {
		other, _ := seed(t, db, flags[0], flags[1], flags[2])
		found, err := repo.FindTemplate(ctx, other, string(edition.Published))
		require.NoError(t, err)
		require.Nil(t, found, name)
	}
}
