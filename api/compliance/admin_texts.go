package compliance

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/compliance"
)

type textVersionJSON struct {
	Version     int    `json:"version"`
	PublishedAt string `json:"published_at"`
}

// textCatalogEntryJSON is one kind in one language. An empty versions list
// is a pair nothing is served for yet.
type textCatalogEntryJSON struct {
	Kind     string            `json:"kind"`
	Language string            `json:"language"`
	Versions []textVersionJSON `json:"versions"`
}

type textCatalogResponse struct {
	Texts []textCatalogEntryJSON `json:"texts"`
}

type publishTextRequest struct {
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

type publishedTextResponse struct {
	Kind        string `json:"kind"`
	Language    string `json:"language"`
	Version     int    `json:"version"`
	PublishedAt string `json:"published_at"`
}

// handleListTexts answers every kind and language with what is published
// for it, so admin sees at a glance which pairs are still missing.
func (h *Handler) handleListTexts(w http.ResponseWriter, r *http.Request) {
	catalog, err := h.texts.AdminListTexts(r.Context())
	if failed(w, r, err) {
		return
	}
	entries := make([]textCatalogEntryJSON, 0, len(catalog))
	for _, entry := range catalog {
		versions := make([]textVersionJSON, 0, len(entry.Versions))
		for _, version := range entry.Versions {
			versions = append(versions, textVersionJSON{
				Version: version.Version, PublishedAt: version.PublishedAt.UTC().Format(time.RFC3339),
			})
		}
		entries = append(entries, textCatalogEntryJSON{Kind: string(entry.Kind), Language: entry.Language, Versions: versions})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, textCatalogResponse{Texts: entries})
}

// handlePublishText publishes a new version of one text. Published versions
// are never edited, so this only ever adds one.
func (h *Handler) handlePublishText(w http.ResponseWriter, r *http.Request) {
	var req publishTextRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTextBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	published, err := h.texts.AdminPublishText(r.Context(), compliance.TextKind(req.Kind), req.Language, req.Content)
	if failed(w, r, err) {
		return
	}
	if published.Outcome == compliance.TextInvalid {
		httpx.WriteCodedError(w, http.StatusBadRequest, string(published.Outcome), "the kind, the language or the content is not valid")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, publishedTextResponse{
		Kind: req.Kind, Language: req.Language, Version: published.Version,
		PublishedAt: published.PublishedAt.UTC().Format(time.RFC3339),
	})
}
