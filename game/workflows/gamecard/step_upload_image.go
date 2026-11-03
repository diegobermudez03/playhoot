package gamecard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/card"
	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamecard/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=gamecard -destination=step_upload_image_mock_test.go . uploadImageRepo

// uploadImageRepo is UploadImage's own narrow persistence contract.
type uploadImageRepo interface {
	FindGame(ctx context.Context, tx *gorm.DB, gameUUID string, lock bool) (*internalrepo.CardGame, error)
	FindActiveImages(ctx context.Context, tx *gorm.DB, gameID uint) ([]internalrepo.Image, error)
	FindImageByID(ctx context.Context, tx *gorm.DB, imageID uint) (*internalrepo.Image, error)
	RegisterObject(ctx context.Context, tx *gorm.DB, sha256 string, size int64, mimeType string, objectKey string, at time.Time) error
	FindImageRequest(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string) (*internalrepo.ImageRequest, error)
	ClaimImageRequest(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string, kind string, sha256 string, at time.Time) (uint, error)
	CompleteImageRequest(ctx context.Context, tx *gorm.DB, requestID uint, imageID uint) error
	CreateImage(ctx context.Context, tx *gorm.DB, gameID uint, image internalrepo.Image) (uint, error)
	RemoveImage(ctx context.Context, tx *gorm.DB, imageID uint, at time.Time) (bool, error)
}

var (
	cardImageOutcomeLabel = metrics.NewLabel("outcome", "done", "unchanged", "not_found", "invalid", "too_large", "too_many_photos", "image_not_found", "replayed")

	cardImageEditsTotal = metrics.NewCounter(
		"game_card_image_edits_total",
		"Uploads and removals of card images, by outcome; replayed counts retries answered from an earlier upload.",
		cardImageOutcomeLabel,
	)
)

// UploadImage receives an image of kind for ownerUUID's Game gameUUID's card,
// cut at its limit, checks it is a picture by its bytes, stores it
// content-addressed before any reference, and shows it as the logo or one
// more photo. Idempotent per (game, idempotencyKey).
func (m *Manager) UploadImage(ctx context.Context, ownerUUID string, gameUUID string, kind card.ImageKind, content io.Reader, idempotencyKey string) (game.CardEdit, error) {
	defer logging.Step(ctx, "gamecard.upload_image").Close()

	if kind != card.Logo && kind != card.Photo {
		return game.CardEdit{}, fmt.Errorf("unknown card image kind %q", kind)
	}
	if idempotencyKey == "" {
		return game.CardEdit{}, fmt.Errorf("idempotency key is required")
	}
	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return game.CardEdit{Outcome: game.CardNotFound}, nil
	}
	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return game.CardEdit{Outcome: game.CardNotFound}, nil
	}
	// The owner is checked before any byte is read.
	found, err := m.uploadImageRepo.FindGame(ctx, m.dbServicer.GetDB(), parsedGame.String(), false)
	if err != nil {
		return game.CardEdit{}, err
	}
	if found == nil || !isOwner(found.OwnerUUID, owner) {
		return countImageEdit(ctx, game.CardEdit{Outcome: game.CardNotFound}, false), nil
	}

	limit := card.MaxImageBytes(kind)
	hash := sha256.New()
	var buffer bytes.Buffer
	read, err := io.Copy(io.MultiWriter(&buffer, hash), io.LimitReader(content, limit+1))
	if err != nil {
		return game.CardEdit{}, fmt.Errorf("reading the uploaded image: %w", err)
	}
	if read > limit {
		return countImageEdit(ctx, game.CardEdit{Outcome: game.CardImageTooLarge, Message: fmt.Sprintf("the image is larger than %d bytes, the limit for a %s", limit, kind)}, false), nil
	}
	mimeType, err := card.ImageType(buffer.Bytes())
	if errors.Is(err, card.ErrInvalidImage) || read == 0 {
		return countImageEdit(ctx, game.CardEdit{Outcome: game.CardInvalid, Message: card.ErrInvalidImage.Error()}, false), nil
	}
	if err != nil {
		return game.CardEdit{}, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	objectKey := imagePrefix + "/" + digest

	now := m.now().UTC().Truncate(time.Microsecond)
	if _, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (struct{}, error) {
		return struct{}{}, m.uploadImageRepo.RegisterObject(ctx, tx, digest, read, mimeType, objectKey, now)
	}); err != nil {
		return game.CardEdit{}, err
	}
	if err := m.images.Put(ctx, objectKey, buffer.Bytes(), mimeType); err != nil {
		return game.CardEdit{}, fmt.Errorf("storing the card image: %w", err)
	}
	uploaded := internalrepo.Image{Kind: string(kind), SHA256: digest, Size: read, MIMEType: mimeType, ObjectKey: objectKey, CreatedAt: now}
	edited, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (imageEdit, error) {
		return m.showImageInTx(ctx, tx, parsedGame.String(), owner, uploaded, idempotencyKey)
	})
	if err != nil {
		return game.CardEdit{}, err
	}
	return countImageEdit(ctx, edited.edit, edited.replayed), nil
}

// imageEdit is what showing an image decided, and whether it replayed an
// earlier upload with the same key.
type imageEdit struct {
	edit     game.CardEdit
	replayed bool
}

func (m *Manager) showImageInTx(ctx context.Context, tx *gorm.DB, gameUUID string, owner uuid.UUID, uploaded internalrepo.Image, idempotencyKey string) (imageEdit, error) {
	found, err := m.uploadImageRepo.FindGame(ctx, tx, gameUUID, true)
	if err != nil {
		return imageEdit{}, err
	}
	if found == nil || !isOwner(found.OwnerUUID, owner) {
		return imageEdit{edit: game.CardEdit{Outcome: game.CardNotFound}}, nil
	}
	request, err := m.uploadImageRepo.FindImageRequest(ctx, tx, found.ID, idempotencyKey)
	if err != nil {
		return imageEdit{}, err
	}
	if request != nil {
		return m.replayImage(ctx, tx, request, uploaded)
	}
	images, err := m.uploadImageRepo.FindActiveImages(ctx, tx, found.ID)
	if err != nil {
		return imageEdit{}, err
	}
	var photos int
	for _, image := range images {
		switch card.ImageKind(image.Kind) {
		case card.Logo:
			if uploaded.Kind != string(card.Logo) {
				continue
			}
			if image.SHA256 == uploaded.SHA256 {
				return imageEdit{edit: game.CardEdit{Outcome: game.CardUnchanged, Image: publicImage(image)}}, nil
			}
			// The new logo replaces the old one.
			if _, err := m.uploadImageRepo.RemoveImage(ctx, tx, image.ID, uploaded.CreatedAt); err != nil {
				return imageEdit{}, err
			}
		case card.Photo:
			photos++
		}
	}
	if uploaded.Kind == string(card.Photo) && photos >= card.MaxPhotos {
		return imageEdit{edit: game.CardEdit{Outcome: game.CardTooManyPhotos, Message: fmt.Sprintf("a card shows at most %d photos", card.MaxPhotos)}}, nil
	}
	requestID, err := m.uploadImageRepo.ClaimImageRequest(ctx, tx, found.ID, idempotencyKey, uploaded.Kind, uploaded.SHA256, uploaded.CreatedAt)
	if err != nil {
		return imageEdit{}, err
	}
	uploaded.UUID = m.newUUID()
	imageID, err := m.uploadImageRepo.CreateImage(ctx, tx, found.ID, uploaded)
	if err != nil {
		return imageEdit{}, err
	}
	if err := m.uploadImageRepo.CompleteImageRequest(ctx, tx, requestID, imageID); err != nil {
		return imageEdit{}, err
	}
	return imageEdit{edit: game.CardEdit{Outcome: game.CardDone, Image: publicImage(uploaded)}}, nil
}

// replayImage answers a retry with the image its request showed.
func (m *Manager) replayImage(ctx context.Context, tx *gorm.DB, request *internalrepo.ImageRequest, uploaded internalrepo.Image) (imageEdit, error) {
	if request.Kind != uploaded.Kind || request.SHA256 != uploaded.SHA256 {
		return imageEdit{}, fmt.Errorf("card image key: %w", game.ErrIdempotencyConflict)
	}
	if request.ImageID == nil {
		monitoring.Alert(ctx, "game card image request has no image")
		return imageEdit{}, fmt.Errorf("card image request %d has no image", request.ID)
	}
	shown, err := m.uploadImageRepo.FindImageByID(ctx, tx, *request.ImageID)
	if err != nil {
		return imageEdit{}, err
	}
	if shown == nil {
		monitoring.Alert(ctx, "game card image request references an image that does not exist")
		return imageEdit{}, fmt.Errorf("card image request %d references a missing image", request.ID)
	}
	return imageEdit{edit: game.CardEdit{Outcome: game.CardDone, Image: publicImage(*shown)}, replayed: true}, nil
}

func publicImage(image internalrepo.Image) *game.CardImage {
	return &game.CardImage{UUID: image.UUID, MIMEType: image.MIMEType, Size: image.Size}
}

func countImageEdit(ctx context.Context, edit game.CardEdit, replayed bool) game.CardEdit {
	outcome := string(edit.Outcome)
	if replayed {
		outcome = "replayed"
	}
	cardImageEditsTotal.Inc(ctx, cardImageOutcomeLabel.Value(outcome))
	logging.LogFields(ctx, logging.Field("card_image_outcome", string(edit.Outcome)))
	return edit
}
