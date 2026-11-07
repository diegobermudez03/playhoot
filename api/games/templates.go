package games

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/discovery"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
)

// templateReader finds and reads templates: anyone signed in may.
type templateReader interface {
	BrowseTemplates(ctx context.Context) (composergameauthoring.TemplateBrowse, error)
	SearchTemplates(ctx context.Context, query discovery.TemplateQuery) (discovery.TemplatePage, error)
	InspectTemplate(ctx context.Context, templateGameUUID string) (composergameauthoring.TemplateRead, error)
	ReadTemplateFiles(ctx context.Context, templateGameUUID string, paths []string) (composergameauthoring.TemplateFiles, error)
}

const (
	codeTemplateNotFound = "template_not_found"
	// maxTemplateFiles bounds one read of a template's files.
	maxTemplateFiles = 50
)

// TemplateEntryResponse is one template as the catalog lists it.
type TemplateEntryResponse struct {
	GameUUID         string   `json:"game_uuid"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Categories       []string `json:"categories"`
	PlayersMin       int      `json:"players_min"`
	PlayersMax       int      `json:"players_max"`
	Languages        []string `json:"languages"`
	AspectRatio      string   `json:"aspect_ratio,omitempty"`
	UsesKeyboard     bool     `json:"uses_keyboard"`
	UsesPointerHover bool     `json:"uses_pointer_hover"`
	Featured         bool     `json:"featured"`
}

// TemplateCategoryResponse is a category and how many templates it holds.
type TemplateCategoryResponse struct {
	Name      string `json:"name"`
	Templates int    `json:"templates"`
}

// BrowseTemplatesResponse is where finding a template starts.
type BrowseTemplatesResponse struct {
	Featured   []TemplateEntryResponse    `json:"featured"`
	Categories []TemplateCategoryResponse `json:"categories"`
}

// SearchTemplatesResponse is one page of a template search.
type SearchTemplatesResponse struct {
	Page      int                     `json:"page"`
	Templates []TemplateEntryResponse `json:"templates"`
	More      bool                    `json:"more"`
}

// TemplateResponse is a template as an AI inspects it: its card, the
// revision it was published at, its brief.json and build manifest as
// written, and its files without contents.
type TemplateResponse struct {
	GameUUID    string              `json:"game_uuid"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Featured    bool                `json:"featured"`
	Revision    int64               `json:"revision"`
	Brief       json.RawMessage     `json:"brief,omitempty"`
	Manifest    json.RawMessage     `json:"manifest,omitempty"`
	Files       []draftFileResponse `json:"files"`
}

// UseTemplateRequest switches a draft to a template over base_revision.
type UseTemplateRequest struct {
	TemplateGameUUID string `json:"template_game_uuid"`
	BaseRevision     *int64 `json:"base_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}

func entriesBody(entries []discovery.TemplateEntry) []TemplateEntryResponse {
	body := make([]TemplateEntryResponse, len(entries))
	for i, entry := range entries {
		categories, languages := entry.Categories, entry.Languages
		if categories == nil {
			categories = []string{}
		}
		if languages == nil {
			languages = []string{}
		}
		body[i] = TemplateEntryResponse{
			GameUUID: entry.GameUUID, Name: entry.Name, Description: entry.Description, Categories: categories,
			PlayersMin: entry.PlayersMin, PlayersMax: entry.PlayersMax, Languages: languages, AspectRatio: entry.AspectRatio,
			UsesKeyboard: entry.UsesKeyboard, UsesPointerHover: entry.UsesPointerHover, Featured: entry.Featured,
		}
	}
	return body
}

// handleBrowseTemplates answers the featured templates and the categories.
func (h *Handler) handleBrowseTemplates(w http.ResponseWriter, r *http.Request) {
	browse, refusal := h.BrowseTemplates(r.Context())
	writeTemplateAnswer(w, browse, refusal)
}

// BrowseTemplates answers the featured templates and the categories: where
// finding a template starts.
func (h *Handler) BrowseTemplates(ctx context.Context) (*BrowseTemplatesResponse, *Refusal) {
	browse, err := h.templates.BrowseTemplates(ctx)
	if err != nil {
		logging.LogError(ctx, err)
		return nil, refuse(http.StatusInternalServerError, "browsing templates failed")
	}
	categories := make([]TemplateCategoryResponse, len(browse.Categories))
	for i, category := range browse.Categories {
		categories[i] = TemplateCategoryResponse{Name: category.Name, Templates: category.Templates}
	}
	return &BrowseTemplatesResponse{Featured: entriesBody(browse.Featured), Categories: categories}, nil
}

// handleSearchTemplates answers a page of templates: ?q=words&category=c&players=n&page=p.
func (h *Handler) handleSearchTemplates(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	search := discovery.TemplateQuery{Text: query.Get("q"), Category: query.Get("category"), Page: 1}
	for name, target := range map[string]*int{"players": &search.Players, "page": &search.Page} {
		raw := query.Get(name)
		if raw == "" {
			continue
		}
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			httpx.WriteError(w, http.StatusBadRequest, name+" must be a positive integer")
			return
		}
		*target = parsed
	}
	page, refusal := h.SearchTemplates(r.Context(), search)
	writeTemplateAnswer(w, page, refusal)
}

// SearchTemplates answers one page of the templates query matches.
func (h *Handler) SearchTemplates(ctx context.Context, query discovery.TemplateQuery) (*SearchTemplatesResponse, *Refusal) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Players < 0 {
		return nil, refuse(http.StatusBadRequest, "players must be a positive integer")
	}
	page, err := h.templates.SearchTemplates(ctx, query)
	if err != nil {
		logging.LogError(ctx, err)
		return nil, refuse(http.StatusInternalServerError, "searching templates failed")
	}
	return &SearchTemplatesResponse{Page: query.Page, Templates: entriesBody(page.Templates), More: page.More}, nil
}

// handleInspectTemplate answers a template as an AI inspects it.
func (h *Handler) handleInspectTemplate(w http.ResponseWriter, r *http.Request) {
	read, refusal := h.InspectTemplate(r.Context(), r.PathValue("templateGameUUID"))
	writeTemplateAnswer(w, read, refusal)
}

// InspectTemplate answers a template's card, brief, manifest and files
// without their contents: enough to choose it, never its code.
func (h *Handler) InspectTemplate(ctx context.Context, templateGameUUID string) (*TemplateResponse, *Refusal) {
	parsed, err := uuid.Parse(templateGameUUID)
	if err != nil {
		return nil, templateNotFound()
	}
	read, err := h.templates.InspectTemplate(ctx, parsed.String())
	if err != nil {
		logging.LogError(ctx, err)
		return nil, refuse(http.StatusInternalServerError, "inspecting the template failed")
	}
	if !read.Found {
		return nil, templateNotFound()
	}
	body := &TemplateResponse{
		GameUUID: read.Template.GameUUID, Name: read.Template.Name, Description: read.Template.Description,
		Featured: read.Template.Featured, Revision: read.Revision, Files: make([]draftFileResponse, len(read.Files)),
	}
	if read.Brief != nil && json.Valid([]byte(*read.Brief)) {
		body.Brief = json.RawMessage(*read.Brief)
	}
	if len(read.Manifest) > 0 && json.Valid(read.Manifest) {
		body.Manifest = json.RawMessage(read.Manifest)
	}
	for i, file := range read.Files {
		body.Files[i] = draftFileResponse{Path: file.Path, SHA256: file.SHA256, Size: file.Size}
	}
	return body, nil
}

// handleReadTemplateFiles answers files of a template: ?path=a&path=b.
func (h *Handler) handleReadTemplateFiles(w http.ResponseWriter, r *http.Request) {
	files, refusal := h.ReadTemplateFiles(r.Context(), r.PathValue("templateGameUUID"), r.URL.Query()["path"])
	writeTemplateAnswer(w, files, refusal)
}

// ReadTemplateFiles answers 1 to 50 files of a template's published revision,
// with their contents, and the paths it does not have.
func (h *Handler) ReadTemplateFiles(ctx context.Context, templateGameUUID string, paths []string) (*FilesResponse, *Refusal) {
	parsed, err := uuid.Parse(templateGameUUID)
	if err != nil {
		return nil, templateNotFound()
	}
	if len(paths) == 0 || len(paths) > maxTemplateFiles {
		return nil, refuse(http.StatusBadRequest, "name 1 to 50 paths")
	}
	read, err := h.templates.ReadTemplateFiles(ctx, parsed.String(), paths)
	if err != nil {
		logging.LogError(ctx, err)
		return nil, refuse(http.StatusInternalServerError, "reading the template's files failed")
	}
	if !read.Found {
		return nil, templateNotFound()
	}
	body := &FilesResponse{Revision: read.Revision, Files: make([]draftFileResponse, len(read.Files)), Missing: read.Missing}
	if body.Missing == nil {
		body.Missing = []string{}
	}
	for i, file := range read.Files {
		body.Files[i] = draftFileResponse{Path: file.Path, SHA256: file.SHA256, Size: file.Size, Content: file.Content}
	}
	return body, nil
}

// handleUseTemplate switches the game's version being built to a template.
func (h *Handler) handleUseTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "using a template")
	if !ok {
		return
	}
	noStore(w)
	var req UseTemplateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateGameBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	response, refusal := h.UseTemplate(ctx, Caller{UserUUID: userUUID, Origin: originOf(ctx)}, gameUUID, req)
	writeChange(w, response, refusal)
}

// UseTemplate makes the game's version being built start over from a
// template as a new revision over base_revision, keeping its notes.md,
// brief.json and the uploads the template still declares.
func (h *Handler) UseTemplate(ctx context.Context, by Caller, rawGameUUID string, req UseTemplateRequest) (ChangeResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return ChangeResponse{}, refusal
	}
	template, err := uuid.Parse(req.TemplateGameUUID)
	if err != nil {
		return ChangeResponse{}, templateNotFound()
	}
	if req.BaseRevision == nil {
		return ChangeResponse{}, refuse(http.StatusBadRequest, "base_revision is required")
	}
	if refusal := idempotencyKeyRefusal(req.IdempotencyKey, "idempotency_key"); refusal != nil {
		return ChangeResponse{}, refusal
	}
	logging.LogFields(ctx, logging.Field("template_game_uuid", template.String()), logging.Field("idempotency_key", req.IdempotencyKey))
	edit, err := h.editor.UseTemplate(ctx, by.UserUUID, gameUUID, template.String(), *req.BaseRevision, req.IdempotencyKey, by.Origin)
	if errors.Is(err, gameauthoring.ErrTemplateNotFound) {
		logging.LogError(ctx, err)
		return ChangeResponse{}, templateNotFound()
	}
	return editAnswer(ctx, edit, err, "using a template")
}

func templateNotFound() *Refusal {
	return refuseCoded(http.StatusNotFound, codeTemplateNotFound, "no published game allowed as a template has that uuid")
}

// writeTemplateAnswer writes a template read, or its refusal.
func writeTemplateAnswer[T any](w http.ResponseWriter, value *T, refusal *Refusal) {
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, value)
}
