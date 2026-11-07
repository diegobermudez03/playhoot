package games

import (
	"net/http"
	"testing"

	"go.uber.org/mock/gomock"

	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/discovery"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const testTemplateGame = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

func TestHandleTemplates(t *testing.T) {
	entry := discovery.TemplateEntry{GameUUID: testTemplateGame, Name: "Quiz", Description: "A quiz", Categories: []string{"trivia"}, PlayersMin: 1, PlayersMax: 30, Languages: []string{"es"}, AspectRatio: "16:9", Featured: true}
	entryJSON := `{"game_uuid":"` + testTemplateGame + `","name":"Quiz","description":"A quiz","categories":["trivia"],"players_min":1,"players_max":30,"languages":["es"],"aspect_ratio":"16:9","uses_keyboard":false,"uses_pointer_hover":false,"featured":true}`

	t.Run("browse", func(t *testing.T) {
		run(t, map[string]func(t *testing.T, deps dependencies) test{
			"featured_and_categories": func(t *testing.T, deps dependencies) test {
				deps.template.EXPECT().BrowseTemplates(gomock.Any()).Return(composergameauthoring.TemplateBrowse{Featured: []discovery.TemplateEntry{entry}, Categories: []discovery.TemplateCategory{{Name: "trivia", Templates: 1}}}, nil)
				return test{method: http.MethodGet, target: "/", expectedCode: http.StatusOK, expectedBody: `{"featured":[` + entryJSON + `],"categories":[{"name":"trivia","templates":1}]}`}
			},
			"an_empty_catalog": func(t *testing.T, deps dependencies) test {
				deps.template.EXPECT().BrowseTemplates(gomock.Any()).Return(composergameauthoring.TemplateBrowse{}, nil)
				return test{method: http.MethodGet, target: "/", expectedCode: http.StatusOK, expectedBody: `{"featured":[],"categories":[]}`}
			},
		}, func(h *Handler) http.HandlerFunc { return h.handleBrowseTemplates })
	})

	t.Run("search", func(t *testing.T) {
		run(t, map[string]func(t *testing.T, deps dependencies) test{
			"words_category_players_and_page": func(t *testing.T, deps dependencies) test {
				deps.template.EXPECT().SearchTemplates(gomock.Any(), discovery.TemplateQuery{Text: "quiz", Category: "trivia", Players: 4, Page: 2}).Return(discovery.TemplatePage{Templates: []discovery.TemplateEntry{entry}, More: true}, nil)
				return test{method: http.MethodGet, target: "/?q=quiz&category=trivia&players=4&page=2", expectedCode: http.StatusOK, expectedBody: `{"page":2,"templates":[` + entryJSON + `],"more":true}`}
			},
			"a_bad_page": func(t *testing.T, deps dependencies) test {
				return test{method: http.MethodGet, target: "/?page=0", expectedCode: http.StatusBadRequest}
			},
		}, func(h *Handler) http.HandlerFunc { return h.handleSearchTemplates })
	})

	t.Run("inspect", func(t *testing.T) {
		brief := `{"format":"playhoot.brief.v1"}`
		run(t, map[string]func(t *testing.T, deps dependencies) test{
			"card_brief_manifest_and_files": func(t *testing.T, deps dependencies) test {
				deps.template.EXPECT().InspectTemplate(gomock.Any(), testTemplateGame).Return(composergameauthoring.TemplateRead{
					Found: true, Template: game.Template{GameUUID: testTemplateGame, Name: "Quiz", Description: "A quiz"}, Revision: 9,
					Brief: &brief, Manifest: []byte(`{"players":{"min":1,"max":30}}`), Files: []sessionpkg.DraftFile{{Path: "server/game.ts", SHA256: "s", Size: 10}},
				}, nil)
				return test{method: http.MethodGet, target: "/", pathValues: map[string]string{"templateGameUUID": testTemplateGame}, expectedCode: http.StatusOK,
					expectedBody: `{"game_uuid":"` + testTemplateGame + `","name":"Quiz","description":"A quiz","featured":false,"revision":9,"brief":{"format":"playhoot.brief.v1"},"manifest":{"players":{"min":1,"max":30}},"files":[{"path":"server/game.ts","sha256":"s","size":10}]}`}
			},
			"with_configuration": func(t *testing.T, deps dependencies) test {
				values := `{"format":"playhoot.config.v1","values":{"rounds":5}}`
				deps.template.EXPECT().InspectTemplate(gomock.Any(), testTemplateGame).Return(composergameauthoring.TemplateRead{
					Found: true, Template: game.Template{GameUUID: testTemplateGame, Name: "Quiz"}, Revision: 9,
					ConfigDeclared: []byte(`{"rounds":{"type":"number","description":"Rounds","default":3}}`), ConfigValues: &values, Files: []sessionpkg.DraftFile{},
				}, nil)
				return test{method: http.MethodGet, target: "/", pathValues: map[string]string{"templateGameUUID": testTemplateGame}, expectedCode: http.StatusOK,
					expectedBody: `{"game_uuid":"` + testTemplateGame + `","name":"Quiz","description":"","featured":false,"revision":9,"config":{"declared":{"rounds":{"type":"number","description":"Rounds","default":3}},"values":{"format":"playhoot.config.v1","values":{"rounds":5}}},"files":[]}`}
			},
			"not_a_template": func(t *testing.T, deps dependencies) test {
				deps.template.EXPECT().InspectTemplate(gomock.Any(), testTemplateGame).Return(composergameauthoring.TemplateRead{}, nil)
				return test{method: http.MethodGet, target: "/", pathValues: map[string]string{"templateGameUUID": testTemplateGame}, expectedCode: http.StatusNotFound,
					expectedBody: `{"code":"template_not_found","message":"no published game allowed as a template has that uuid"}`}
			},
		}, func(h *Handler) http.HandlerFunc { return h.handleInspectTemplate })
	})

	t.Run("files", func(t *testing.T) {
		code := "export default 1"
		run(t, map[string]func(t *testing.T, deps dependencies) test{
			"some_files": func(t *testing.T, deps dependencies) test {
				deps.template.EXPECT().ReadTemplateFiles(gomock.Any(), testTemplateGame, []string{"server/game.ts", "x.ts"}).Return(composergameauthoring.TemplateFiles{
					Found: true, Revision: 9, Files: []sessionpkg.DraftFile{{Path: "server/game.ts", SHA256: "s", Size: 16, Content: &code}}, Missing: []string{"x.ts"},
				}, nil)
				return test{method: http.MethodGet, target: "/?path=server/game.ts&path=x.ts", pathValues: map[string]string{"templateGameUUID": testTemplateGame}, expectedCode: http.StatusOK,
					expectedBody: `{"revision":9,"files":[{"path":"server/game.ts","sha256":"s","size":16,"content":"export default 1"}],"missing":["x.ts"]}`}
			},
			"no_path": func(t *testing.T, deps dependencies) test {
				return test{method: http.MethodGet, target: "/", pathValues: map[string]string{"templateGameUUID": testTemplateGame}, expectedCode: http.StatusBadRequest}
			},
		}, func(h *Handler) http.HandlerFunc { return h.handleReadTemplateFiles })
	})
}

func TestHandleUseTemplate(t *testing.T) {
	post := func(body string) test {
		return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, body: body, noStore: true}
	}
	found := game.EditableVersionFound
	run(t, map[string]func(t *testing.T, deps dependencies) test{
		"switched": func(t *testing.T, deps dependencies) test {
			deps.editor.EXPECT().UseTemplate(gomock.Any(), callerUUID, gameUUID, testTemplateGame, int64(3), "k", sessionpkg.DraftOriginWeb).
				Return(gameauthoring.DraftEdit{Access: found, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 4}}, nil)
			tc := post(`{"template_game_uuid":"` + testTemplateGame + `","base_revision":3,"idempotency_key":"k"}`)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"outcome":"applied","revision":4}`
			return tc
		},
		"not_a_template": func(t *testing.T, deps dependencies) test {
			deps.editor.EXPECT().UseTemplate(gomock.Any(), callerUUID, gameUUID, testTemplateGame, int64(3), "k", sessionpkg.DraftOriginWeb).Return(gameauthoring.DraftEdit{}, gameauthoring.ErrTemplateNotFound)
			tc := post(`{"template_game_uuid":"` + testTemplateGame + `","base_revision":3,"idempotency_key":"k"}`)
			tc.expectedCode = http.StatusNotFound
			return tc
		},
		"stale_base": func(t *testing.T, deps dependencies) test {
			deps.editor.EXPECT().UseTemplate(gomock.Any(), callerUUID, gameUUID, testTemplateGame, int64(3), "k", sessionpkg.DraftOriginWeb).
				Return(gameauthoring.DraftEdit{Access: found, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: 5}}, nil)
			tc := post(`{"template_game_uuid":"` + testTemplateGame + `","base_revision":3,"idempotency_key":"k"}`)
			tc.expectedCode = http.StatusConflict
			return tc
		},
		"no_base_revision": func(t *testing.T, deps dependencies) test {
			tc := post(`{"template_game_uuid":"` + testTemplateGame + `","idempotency_key":"k"}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
	}, func(h *Handler) http.HandlerFunc { return h.handleUseTemplate })
}

func TestCreateGameFromATemplate(t *testing.T) {
	run(t, map[string]func(t *testing.T, deps dependencies) test{
		"from_a_template": func(t *testing.T, deps dependencies) test {
			deps.creator.EXPECT().CreateGame(gomock.Any(), callerUUID, "Quiz", "key-1", testTemplateGame).Return(gameauthoring.CreatedGame{GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 1}, nil)
			return test{method: http.MethodPost, target: "/v1/games", body: `{"name":"Quiz","idempotency_key":"key-1","template_game_uuid":"` + testTemplateGame + `"}`, expectedCode: http.StatusCreated}
		},
		"not_a_template": func(t *testing.T, deps dependencies) test {
			deps.creator.EXPECT().CreateGame(gomock.Any(), callerUUID, "Quiz", "key-1", testTemplateGame).Return(gameauthoring.CreatedGame{}, gameauthoring.ErrTemplateNotFound)
			return test{method: http.MethodPost, target: "/v1/games", body: `{"name":"Quiz","idempotency_key":"key-1","template_game_uuid":"` + testTemplateGame + `"}`, expectedCode: http.StatusNotFound,
				expectedBody: `{"code":"template_not_found","message":"no published game allowed as a template has that uuid"}`}
		},
	}, func(h *Handler) http.HandlerFunc { return h.handleCreateGame })
}
