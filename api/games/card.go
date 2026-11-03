package games

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
)

// cardManager is Game Management's card of a game: read and changed by its
// owner, texts and images.
type cardManager interface {
	ReadCard(ctx context.Context, ownerUUID string, gameUUID string) (game.CardRead, error)
	UpdateCard(ctx context.Context, ownerUUID string, gameUUID string, change game.CardChange) (game.CardEdit, error)
	UploadImage(ctx context.Context, ownerUUID string, gameUUID string, kind game.CardImageKind, content io.Reader, idempotencyKey string) (game.CardEdit, error)
	RemoveImage(ctx context.Context, ownerUUID string, gameUUID string, kind game.CardImageKind, imageUUID string) (game.CardEdit, error)
}

// publisher publishes a game and opens a new edition of a published one.
type publisher interface {
	Publish(ctx context.Context, userUUID string, gameUUID string, visibility game.VisibilityType, name *string, idempotencyKey string) (gameauthoring.Publication, error)
	OpenEdition(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (gameauthoring.EditionOpening, error)
}

// Cards is what a game's card and its publication need.
type Cards struct {
	Card      cardManager
	Publisher publisher
}

const (
	maxCardBodyBytes = 16 << 10
	// maxCardImageBodyBytes lets the largest card image through; its kind's
	// own limit is checked while reading.
	maxCardImageBodyBytes = 5<<20 + 1
	cardImageReadTime     = 2 * time.Minute

	codeInvalidCard    = "invalid_card"
	codeImageTooLarge  = "image_too_large"
	codeTooManyPhotos  = "too_many_photos"
	codeImageNotFound  = "image_not_found"
	codeNotReady       = "not_ready"
	codeNotPublished   = "not_published"
	codeEditionOpen    = "edition_open"
	codePublishing     = "publishing"
	codeInvalidRequest = "invalid_request"
)

type cardImageResponse struct {
	UUID         string     `json:"uuid"`
	MIMEType     string     `json:"mime_type"`
	Size         int64      `json:"size"`
	URL          string     `json:"url,omitempty"`
	URLExpiresAt *time.Time `json:"url_expires_at,omitempty"`
}

type cardResponse struct {
	GameUUID    string              `json:"game_uuid"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Logo        *cardImageResponse  `json:"logo"`
	Photos      []cardImageResponse `json:"photos"`
	Visibility  string              `json:"visibility"`
	Published   bool                `json:"published"`
	PublishedAt *time.Time          `json:"published_at,omitempty"`
	Editing     bool                `json:"editing"`
	Publishing  bool                `json:"publishing"`
	Live        *liveResponse       `json:"live"`
}

// liveResponse is the publication players get.
type liveResponse struct {
	ReleaseSequence int64     `json:"release_sequence"`
	Revision        int64     `json:"revision"`
	VersionUUID     string    `json:"version_uuid"`
	PublishedAt     time.Time `json:"published_at"`
}

func (h *Handler) handleReadCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "reading card")
	if !ok {
		return
	}
	noStore(w)
	read, err := h.cards.Card.ReadCard(ctx, userUUID, gameUUID)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "reading card failed")
		return
	}
	if read.Outcome != game.CardDone {
		httpx.WriteError(w, http.StatusNotFound, "game not found")
		return
	}
	card := read.Card
	response := cardResponse{
		GameUUID: card.GameUUID, Name: card.Name, Description: card.Description, Photos: []cardImageResponse{},
		Visibility: string(card.Visibility), Published: card.Published, PublishedAt: card.PublishedAt,
		Editing: card.Editing, Publishing: card.Publishing,
	}
	if card.Live != nil {
		response.Live = &liveResponse{ReleaseSequence: card.Live.Sequence, Revision: card.Live.Revision, VersionUUID: card.Live.VersionUUID, PublishedAt: card.Live.PublishedAt}
	}
	if card.Logo != nil {
		logo := imageResponse(*card.Logo)
		response.Logo = &logo
	}
	for _, photo := range card.Photos {
		response.Photos = append(response.Photos, imageResponse(photo))
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func imageResponse(image game.CardImage) cardImageResponse {
	shown := cardImageResponse{UUID: image.UUID, MIMEType: image.MIMEType, Size: image.Size, URL: image.URL}
	if !image.URLExpiresAt.IsZero() {
		expires := image.URLExpiresAt
		shown.URLExpiresAt = &expires
	}
	return shown
}

type updateCardRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (h *Handler) handleUpdateCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "updating card")
	if !ok {
		return
	}
	noStore(w)
	var req updateCardRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCardBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	edit, err := h.cards.Card.UpdateCard(ctx, userUUID, gameUUID, game.CardChange{Name: req.Name, Description: req.Description})
	writeCardEdit(ctx, w, edit, err, "updating card")
}

func (h *Handler) handleUploadLogo(w http.ResponseWriter, r *http.Request) {
	h.uploadCardImage(w, r, game.CardLogo)
}

func (h *Handler) handleAddPhoto(w http.ResponseWriter, r *http.Request) {
	h.uploadCardImage(w, r, game.CardPhoto)
}

func (h *Handler) uploadCardImage(w http.ResponseWriter, r *http.Request, kind game.CardImageKind) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "uploading card image")
	if !ok {
		return
	}
	noStore(w)
	idempotencyKey := r.Header.Get("Idempotency-Key")
	logging.LogFields(ctx, logging.Field("image_kind", string(kind)), logging.Field("idempotency_key", idempotencyKey))
	if idempotencyKey == "" || len(idempotencyKey) > maxIdempotencyKeyLength {
		httpx.WriteError(w, http.StatusBadRequest, "the Idempotency-Key header is required and must be at most 128 characters")
		return
	}
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(cardImageReadTime)); err != nil {
		logging.LogError(ctx, err)
	}
	body := http.MaxBytesReader(w, r.Body, maxCardImageBodyBytes)
	edit, err := h.cards.Card.UploadImage(ctx, userUUID, gameUUID, kind, body, idempotencyKey)
	writeCardEdit(ctx, w, edit, err, "uploading card image")
}

func (h *Handler) handleRemoveLogo(w http.ResponseWriter, r *http.Request) {
	h.removeCardImage(w, r, game.CardLogo, "")
}

func (h *Handler) handleRemovePhoto(w http.ResponseWriter, r *http.Request) {
	h.removeCardImage(w, r, game.CardPhoto, r.PathValue("photoID"))
}

func (h *Handler) removeCardImage(w http.ResponseWriter, r *http.Request, kind game.CardImageKind, imageUUID string) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "removing card image")
	if !ok {
		return
	}
	noStore(w)
	edit, err := h.cards.Card.RemoveImage(ctx, userUUID, gameUUID, kind, imageUUID)
	writeCardEdit(ctx, w, edit, err, "removing card image")
}

type cardEditResponse struct {
	Outcome string             `json:"outcome"`
	Image   *cardImageResponse `json:"image,omitempty"`
}

func writeCardEdit(ctx context.Context, w http.ResponseWriter, edit game.CardEdit, err error, what string) {
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, game.ErrIdempotencyConflict) {
			httpx.WriteCodedError(w, http.StatusConflict, codeIdempotencyConflict, "idempotency key reused with a conflicting request")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, what+" failed")
		return
	}
	logging.LogFields(ctx, logging.Field("card_outcome", string(edit.Outcome)))
	switch edit.Outcome {
	case game.CardDone, game.CardUnchanged:
		response := cardEditResponse{Outcome: string(edit.Outcome)}
		if edit.Image != nil {
			image := imageResponse(*edit.Image)
			response.Image = &image
		}
		httpx.WriteJSON(w, http.StatusOK, response)
	case game.CardNotFound:
		httpx.WriteError(w, http.StatusNotFound, "game not found")
	case game.CardInvalid:
		httpx.WriteCodedError(w, http.StatusUnprocessableEntity, codeInvalidCard, edit.Message)
	case game.CardImageTooLarge:
		httpx.WriteCodedError(w, http.StatusRequestEntityTooLarge, codeImageTooLarge, edit.Message)
	case game.CardTooManyPhotos:
		httpx.WriteCodedError(w, http.StatusConflict, codeTooManyPhotos, edit.Message)
	case game.CardImageNotFound:
		httpx.WriteCodedError(w, http.StatusNotFound, codeImageNotFound, "the card shows no such image")
	default:
		logging.LogFields(ctx, logging.Field("unknown_card_outcome", string(edit.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, what+" failed")
	}
}

type publishRequest struct {
	Visibility     string  `json:"visibility"`
	Name           *string `json:"name"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type releaseResponse struct {
	ReleaseUUID string `json:"release_uuid"`
	Sequence    int64  `json:"sequence"`
	VersionUUID string `json:"version_uuid"`
	Revision    int64  `json:"revision"`
	Visibility  string `json:"visibility"`
}

type publishResponse struct {
	Outcome string `json:"outcome"`
	releaseResponse
}

type blockerResponse struct {
	Code        string   `json:"code"`
	BuildStatus string   `json:"build_status,omitempty"`
	Keys        []string `json:"keys,omitempty"`
}

func (h *Handler) handlePublish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "publishing game")
	if !ok {
		return
	}
	noStore(w)
	var req publishRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCardBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logging.LogFields(ctx, logging.Field("visibility", req.Visibility), logging.Field("idempotency_key", req.IdempotencyKey))
	visibility := game.VisibilityType(req.Visibility)
	if visibility != game.Public && visibility != game.Private {
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalidRequest, "visibility must be public or private")
		return
	}
	if req.IdempotencyKey == "" || len(req.IdempotencyKey) > maxIdempotencyKeyLength {
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required and must be at most 128 characters")
		return
	}
	published, err := h.cards.Publisher.Publish(ctx, userUUID, gameUUID, visibility, req.Name, req.IdempotencyKey)
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, game.ErrIdempotencyConflict) {
			httpx.WriteCodedError(w, http.StatusConflict, codeIdempotencyConflict, "idempotency key reused with a conflicting request")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "publishing game failed")
		return
	}
	logging.LogFields(ctx, logging.Field("publish_outcome", string(published.Outcome)))
	switch published.Outcome {
	case gameauthoring.Published:
		if published.Release == nil {
			httpx.WriteError(w, http.StatusInternalServerError, "publishing game failed")
			return
		}
		release := published.Release
		httpx.WriteJSON(w, http.StatusOK, publishResponse{Outcome: string(published.Outcome), releaseResponse: releaseResponse{
			ReleaseUUID: release.UUID, Sequence: release.Sequence, VersionUUID: release.VersionUUID,
			Revision: release.Revision, Visibility: string(release.Visibility),
		}})
	case gameauthoring.PublishNotReady:
		blockers := make([]blockerResponse, len(published.Blockers))
		for i, blocker := range published.Blockers {
			blockers[i] = blockerResponse{Code: blocker.Code, BuildStatus: blocker.BuildStatus, Keys: blocker.Keys}
		}
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{"code": codeNotReady, "message": "the game is not ready to be published", "blockers": blockers})
	case gameauthoring.PublishNotEditable:
		httpx.WriteCodedError(w, http.StatusConflict, codeNotEditable, "the game has no edition open to publish")
	case gameauthoring.PublishNotFound:
		httpx.WriteError(w, http.StatusNotFound, "game not found")
	default:
		logging.LogFields(ctx, logging.Field("unknown_publish_outcome", string(published.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "publishing game failed")
	}
}

type openEditionRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type openEditionResponse struct {
	VersionUUID string `json:"version_uuid"`
	Revision    int64  `json:"revision"`
}

func (h *Handler) handleOpenEdition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "opening edition")
	if !ok {
		return
	}
	noStore(w)
	var req openEditionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCardBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.IdempotencyKey == "" || len(req.IdempotencyKey) > maxIdempotencyKeyLength {
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required and must be at most 128 characters")
		return
	}
	opened, err := h.cards.Publisher.OpenEdition(ctx, userUUID, gameUUID, req.IdempotencyKey)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "opening edition failed")
		return
	}
	switch opened.Outcome {
	case game.EditionOpened:
		httpx.WriteJSON(w, http.StatusCreated, openEditionResponse{VersionUUID: opened.VersionUUID, Revision: opened.Revision})
	case game.EditionNotFound:
		httpx.WriteError(w, http.StatusNotFound, "game not found")
	case game.EditionNotPublished:
		httpx.WriteCodedError(w, http.StatusConflict, codeNotPublished, "the game was never published: keep editing its first edition")
	case game.EditionAlreadyOpen:
		httpx.WriteCodedError(w, http.StatusConflict, codeEditionOpen, "the game already has an edition open")
	case game.EditionPublishing:
		httpx.WriteCodedError(w, http.StatusConflict, codePublishing, "the game is being published; try again in a moment")
	default:
		logging.LogFields(ctx, logging.Field("unknown_edition_outcome", string(opened.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "opening edition failed")
	}
}
