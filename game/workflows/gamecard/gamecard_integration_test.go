package gamecard

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/card"
	"github.com/diegobermudez03/playhoot/game/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/workflows/gamelifecycle"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

// memoryStore keeps card images in memory and signs fake links.
type memoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (s *memoryStore) Put(ctx context.Context, key string, content []byte, contentType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = append([]byte(nil), content...)
	return nil
}

func (s *memoryStore) PresignGet(ctx context.Context, key string, expiresAt time.Time) (string, error) {
	return "signed://" + key, nil
}

var (
	pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01")
	gifBytes = []byte("GIF89a\x01\x00\x01\x00")
)

func TestCardIntegration(t *testing.T) {
	db := testdb.OpenGameDB(t)
	store := &memoryStore{objects: map[string][]byte{}}
	cards := New(db, store)
	lifecycle := gamelifecycle.New(db)
	ctx := context.Background()

	newGame := func(t *testing.T) (string, string) {
		t.Helper()
		owner := uuid.NewString()
		created, err := lifecycle.CreateGame(ctx, owner, "Memoria", "create")
		require.NoError(t, err)
		return owner, created.GameUUID
	}
	text := func(s string) *string { return &s }

	t.Run("texts change within their limits and are recorded in history", func(t *testing.T) {
		owner, gameUUID := newGame(t)
		edited, err := cards.UpdateCard(ctx, owner, gameUUID, game.CardChange{Name: text("  Memoria Plus "), Description: text("Encuentra las parejas")})
		require.NoError(t, err)
		require.Equal(t, game.CardDone, edited.Outcome)
		same, err := cards.UpdateCard(ctx, owner, gameUUID, game.CardChange{Name: text("Memoria Plus")})
		require.NoError(t, err)
		require.Equal(t, game.CardUnchanged, same.Outcome)
		tooLong, err := cards.UpdateCard(ctx, owner, gameUUID, game.CardChange{Name: text(strings.Repeat("x", card.MaxNameRunes+1))})
		require.NoError(t, err)
		require.Equal(t, game.CardInvalid, tooLong.Outcome)
		stranger, err := cards.UpdateCard(ctx, uuid.NewString(), gameUUID, game.CardChange{Name: text("Mine")})
		require.NoError(t, err)
		require.Equal(t, game.CardNotFound, stranger.Outcome)

		read, err := cards.ReadCard(ctx, owner, gameUUID)
		require.NoError(t, err)
		require.Equal(t, "Memoria Plus", read.Card.Name)
		require.Equal(t, "Encuentra las parejas", read.Card.Description)
		require.True(t, read.Card.Editing)
		require.False(t, read.Card.Published)
		var changes int
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM game_histories h JOIN games g ON g.id = h.game_id WHERE g.uuid = ? AND h.name IS NOT NULL`, gameUUID).Scan(&changes).Error)
		require.Equal(t, 2, changes, "the creation and the one change of name")
	})

	t.Run("a logo is checked by its bytes, stored and replaced", func(t *testing.T) {
		owner, gameUUID := newGame(t)
		wrong, err := cards.UploadImage(ctx, owner, gameUUID, card.Logo, bytes.NewReader(gifBytes), "gif")
		require.NoError(t, err)
		require.Equal(t, game.CardInvalid, wrong.Outcome)
		large, err := cards.UploadImage(ctx, owner, gameUUID, card.Logo, bytes.NewReader(append(pngBytes, make([]byte, card.MaxImageBytes(card.Logo))...)), "large")
		require.NoError(t, err)
		require.Equal(t, game.CardImageTooLarge, large.Outcome)

		logo, err := cards.UploadImage(ctx, owner, gameUUID, card.Logo, bytes.NewReader(pngBytes), "logo")
		require.NoError(t, err)
		require.Equal(t, game.CardDone, logo.Outcome)
		retried, err := cards.UploadImage(ctx, owner, gameUUID, card.Logo, bytes.NewReader(pngBytes), "logo")
		require.NoError(t, err)
		require.Equal(t, logo.Image.UUID, retried.Image.UUID)
		_, err = cards.UploadImage(ctx, owner, gameUUID, card.Logo, bytes.NewReader(append(pngBytes, 'x')), "logo")
		require.ErrorIs(t, err, game.ErrIdempotencyConflict)
		same, err := cards.UploadImage(ctx, owner, gameUUID, card.Logo, bytes.NewReader(pngBytes), "logo-again")
		require.NoError(t, err)
		require.Equal(t, game.CardUnchanged, same.Outcome)

		replacement, err := cards.UploadImage(ctx, owner, gameUUID, card.Logo, bytes.NewReader(append(pngBytes, 'y')), "replace")
		require.NoError(t, err)
		require.Equal(t, game.CardDone, replacement.Outcome)
		read, err := cards.ReadCard(ctx, owner, gameUUID)
		require.NoError(t, err)
		require.Equal(t, replacement.Image.UUID, read.Card.Logo.UUID)
		require.True(t, strings.HasPrefix(read.Card.Logo.URL, "signed://"+imagePrefix+"/"))

		removed, err := cards.RemoveImage(ctx, owner, gameUUID, card.Logo, "")
		require.NoError(t, err)
		require.Equal(t, game.CardDone, removed.Outcome)
		gone, err := cards.RemoveImage(ctx, owner, gameUUID, card.Logo, "")
		require.NoError(t, err)
		require.Equal(t, game.CardImageNotFound, gone.Outcome)
	})

	t.Run("a card shows at most its photos", func(t *testing.T) {
		owner, gameUUID := newGame(t)
		var first string
		for i := 0; i < card.MaxPhotos; i++ {
			added, err := cards.UploadImage(ctx, owner, gameUUID, card.Photo, bytes.NewReader(append(pngBytes, byte(i))), "photo-"+string(rune('a'+i)))
			require.NoError(t, err)
			require.Equal(t, game.CardDone, added.Outcome)
			if i == 0 {
				first = added.Image.UUID
			}
		}
		full, err := cards.UploadImage(ctx, owner, gameUUID, card.Photo, bytes.NewReader(append(pngBytes, 'z')), "one-more")
		require.NoError(t, err)
		require.Equal(t, game.CardTooManyPhotos, full.Outcome)
		removed, err := cards.RemoveImage(ctx, owner, gameUUID, card.Photo, first)
		require.NoError(t, err)
		require.Equal(t, game.CardDone, removed.Outcome)
		read, err := cards.ReadCard(ctx, owner, gameUUID)
		require.NoError(t, err)
		require.Len(t, read.Card.Photos, card.MaxPhotos-1)
	})
}
