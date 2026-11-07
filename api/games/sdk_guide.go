package games

import (
	"context"
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// sdkGuide is Play's SDK guide for agents, from the toolchain it builds with.
type sdkGuide interface {
	ReadPage(ctx context.Context, page string) (sessionpkg.SDKGuidePage, error)
}

const (
	codeSDKPageNotFound      = "sdk_page_not_found"
	codeSDKGuideUnavailable  = "sdk_guide_unavailable"
	toolchainVersionHeader   = "Playhoot-Toolchain-Version"
	sdkPageContentType       = "text/markdown; charset=utf-8"
	sdkGuideUnavailableNotes = "the SDK guide is not available right now"
)

// SDKPageResponse is one page of the SDK guide, in Markdown.
type SDKPageResponse struct {
	Page             string
	Content          string
	ToolchainVersion string
}

// handleAISDKPage answers a page of the SDK guide (index.md by default) as
// Markdown, with the toolchain's version in a header.
func (h *Handler) handleAISDKPage(w http.ResponseWriter, r *http.Request) {
	page := r.PathValue("page")
	if page == "" {
		page = sessionpkg.SDKGuideStart
	}
	read, refusal := h.ReadSDKPage(r.Context(), page)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	w.Header().Set("Content-Type", sdkPageContentType)
	if read.ToolchainVersion != "" {
		w.Header().Set(toolchainVersionHeader, read.ToolchainVersion)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(read.Content))
}

// ReadSDKPage reads one page of the SDK guide that belongs to the toolchain
// games are built with: capabilities.md is the compact index, index.md where
// it starts.
func (h *Handler) ReadSDKPage(ctx context.Context, page string) (*SDKPageResponse, *Refusal) {
	if h.links.Guide == nil {
		return nil, refuseCoded(http.StatusServiceUnavailable, codeSDKGuideUnavailable, sdkGuideUnavailableNotes)
	}
	read, err := h.links.Guide.ReadPage(ctx, page)
	if err != nil {
		logging.LogError(ctx, err)
		switch {
		case errors.Is(err, sessionpkg.ErrSDKGuidePageNotFound):
			return nil, refuseCoded(http.StatusNotFound, codeSDKPageNotFound, "the guide has no such page; capabilities.md lists them")
		case errors.Is(err, sessionpkg.ErrSDKGuideUnavailable):
			return nil, refuseCoded(http.StatusServiceUnavailable, codeSDKGuideUnavailable, sdkGuideUnavailableNotes)
		default:
			return nil, refuse(http.StatusInternalServerError, "reading the SDK guide failed")
		}
	}
	return &SDKPageResponse{Page: read.Name, Content: read.Content, ToolchainVersion: read.ToolchainVersion}, nil
}
