package games

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
)

func TestHandleReadCard(t *testing.T) {
	expires := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"the_owner_reads_the_card": func(t *testing.T, deps dependencies) test {
			deps.card.EXPECT().ReadCard(gomock.Any(), callerUUID, gameUUID).Return(game.CardRead{Outcome: game.CardDone, Card: game.Card{
				GameUUID: gameUUID, Name: "Memoria", Visibility: game.Private, Editing: true,
				Logo:   &game.CardImage{UUID: "l-1", MIMEType: "image/png", Size: 3, URL: "https://signed/logo", URLExpiresAt: expires},
				Photos: []game.CardImage{},
			}}, nil)
			return test{method: http.MethodGet, target: "/", gameUUID: gameUUID, noStore: true, expectedCode: http.StatusOK, expectedBody: `{
				"game_uuid":"` + gameUUID + `","name":"Memoria","description":"","visibility":"private","published":false,"editing":true,"publishing":false,
				"logo":{"uuid":"l-1","mime_type":"image/png","size":3,"url":"https://signed/logo","url_expires_at":"2026-11-03T10:00:00Z"},"photos":[]}`}
		},
		"someone_else": func(t *testing.T, deps dependencies) test {
			deps.card.EXPECT().ReadCard(gomock.Any(), callerUUID, gameUUID).Return(game.CardRead{Outcome: game.CardNotFound}, nil)
			return test{method: http.MethodGet, target: "/", gameUUID: gameUUID, expectedCode: http.StatusNotFound}
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleReadCard })
}

func TestHandleUpdateCardAndImages(t *testing.T) {
	name := "Memoria"
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"texts_change": func(t *testing.T, deps dependencies) test {
			deps.card.EXPECT().UpdateCard(gomock.Any(), callerUUID, gameUUID, game.CardChange{Name: &name}).Return(game.CardEdit{Outcome: game.CardDone}, nil)
			return test{method: http.MethodPatch, target: "/", gameUUID: gameUUID, body: `{"name":"Memoria"}`, expectedCode: http.StatusOK, expectedBody: `{"outcome":"done"}`}
		},
		"a_text_too_long": func(t *testing.T, deps dependencies) test {
			deps.card.EXPECT().UpdateCard(gomock.Any(), callerUUID, gameUUID, game.CardChange{Name: &name}).Return(game.CardEdit{Outcome: game.CardInvalid, Message: "too long"}, nil)
			return test{method: http.MethodPatch, target: "/", gameUUID: gameUUID, body: `{"name":"Memoria"}`, expectedCode: http.StatusUnprocessableEntity, expectedBody: `{"code":"invalid_card","message":"too long"}`}
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleUpdateCard })

	uploads := map[string]func(t *testing.T, deps dependencies) test{
		"a_logo_is_shown": func(t *testing.T, deps dependencies) test {
			deps.card.EXPECT().UploadImage(gomock.Any(), callerUUID, gameUUID, game.CardLogo, gomock.Any(), "k1").Return(game.CardEdit{Outcome: game.CardDone, Image: &game.CardImage{UUID: "l-1", MIMEType: "image/png", Size: 3}}, nil)
			return test{method: http.MethodPut, target: "/", gameUUID: gameUUID, body: "png", headers: map[string]string{"Idempotency-Key": "k1"}, expectedCode: http.StatusOK, expectedBody: `{"outcome":"done","image":{"uuid":"l-1","mime_type":"image/png","size":3}}`}
		},
		"too_large": func(t *testing.T, deps dependencies) test {
			deps.card.EXPECT().UploadImage(gomock.Any(), callerUUID, gameUUID, game.CardLogo, gomock.Any(), "k1").Return(game.CardEdit{Outcome: game.CardImageTooLarge, Message: "big"}, nil)
			return test{method: http.MethodPut, target: "/", gameUUID: gameUUID, body: "png", headers: map[string]string{"Idempotency-Key": "k1"}, expectedCode: http.StatusRequestEntityTooLarge}
		},
		"a_reused_key": func(t *testing.T, deps dependencies) test {
			deps.card.EXPECT().UploadImage(gomock.Any(), callerUUID, gameUUID, game.CardLogo, gomock.Any(), "k1").Return(game.CardEdit{}, game.ErrIdempotencyConflict)
			return test{method: http.MethodPut, target: "/", gameUUID: gameUUID, body: "png", headers: map[string]string{"Idempotency-Key": "k1"}, expectedCode: http.StatusConflict}
		},
		"no_key": func(t *testing.T, deps dependencies) test {
			return test{method: http.MethodPut, target: "/", gameUUID: gameUUID, body: "png", expectedCode: http.StatusBadRequest}
		},
	}
	run(t, uploads, func(h *Handler) http.HandlerFunc { return h.handleUploadLogo })
}

func TestHandlePublish(t *testing.T) {
	release := &game.Release{UUID: "r-1", Sequence: 1, VersionUUID: "v-1", Revision: 3, Visibility: game.Public}
	post := func(body string) test {
		return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, body: body, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"published": func(t *testing.T, deps dependencies) test {
			deps.pub.EXPECT().Publish(gomock.Any(), callerUUID, gameUUID, game.Public, gomock.Nil(), "k1").Return(gameauthoring.Publication{Outcome: gameauthoring.Published, Release: release}, nil)
			tc := post(`{"visibility":"public","idempotency_key":"k1"}`)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"outcome":"published","release_uuid":"r-1","sequence":1,"version_uuid":"v-1","revision":3,"visibility":"public"}`
			return tc
		},
		"not_ready": func(t *testing.T, deps dependencies) test {
			deps.pub.EXPECT().Publish(gomock.Any(), callerUUID, gameUUID, game.Private, gomock.Nil(), "k1").Return(gameauthoring.Publication{Outcome: gameauthoring.PublishNotReady, Blockers: []gameauthoring.PublishBlocker{
				{Code: "name_required"}, {Code: "missing_required_assets", Keys: []string{"cardBack"}},
			}}, nil)
			tc := post(`{"visibility":"private","idempotency_key":"k1"}`)
			tc.expectedCode = http.StatusConflict
			tc.expectedBody = `{"code":"not_ready","message":"the game is not ready to be published","blockers":[{"code":"name_required"},{"code":"missing_required_assets","keys":["cardBack"]}]}`
			return tc
		},
		"not_editable": func(t *testing.T, deps dependencies) test {
			deps.pub.EXPECT().Publish(gomock.Any(), callerUUID, gameUUID, game.Private, gomock.Nil(), "k1").Return(gameauthoring.Publication{Outcome: gameauthoring.PublishNotEditable}, nil)
			tc := post(`{"visibility":"private","idempotency_key":"k1"}`)
			tc.expectedCode = http.StatusConflict
			return tc
		},
		"published_with_a_name": func(t *testing.T, deps dependencies) test {
			name := "Memoria"
			deps.pub.EXPECT().Publish(gomock.Any(), callerUUID, gameUUID, game.Public, &name, "k1").Return(gameauthoring.Publication{Outcome: gameauthoring.Published, Release: release}, nil)
			tc := post(`{"visibility":"public","name":"Memoria","idempotency_key":"k1"}`)
			tc.expectedCode = http.StatusOK
			return tc
		},
		"an_unknown_visibility": func(t *testing.T, deps dependencies) test {
			tc := post(`{"visibility":"draft","idempotency_key":"k1"}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"failure": func(t *testing.T, deps dependencies) test {
			deps.pub.EXPECT().Publish(gomock.Any(), callerUUID, gameUUID, game.Private, gomock.Nil(), "k1").Return(gameauthoring.Publication{}, errors.New("down"))
			tc := post(`{"visibility":"private","idempotency_key":"k1"}`)
			tc.expectedCode = http.StatusInternalServerError
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handlePublish })
}

func TestHandleOpenEdition(t *testing.T) {
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"opened": func(t *testing.T, deps dependencies) test {
			deps.pub.EXPECT().OpenEdition(gomock.Any(), callerUUID, gameUUID, "k1").Return(gameauthoring.EditionOpening{Outcome: game.EditionOpened, VersionUUID: "v-2", Revision: 1}, nil)
			return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, body: `{"idempotency_key":"k1"}`, expectedCode: http.StatusCreated, expectedBody: `{"version_uuid":"v-2","revision":1}`}
		},
		"never_published": func(t *testing.T, deps dependencies) test {
			deps.pub.EXPECT().OpenEdition(gomock.Any(), callerUUID, gameUUID, "k1").Return(gameauthoring.EditionOpening{Outcome: game.EditionNotPublished}, nil)
			return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, body: `{"idempotency_key":"k1"}`, expectedCode: http.StatusConflict, expectedBody: `{"code":"not_published","message":"the game was never published: keep editing its first edition"}`}
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleOpenEdition })
}
