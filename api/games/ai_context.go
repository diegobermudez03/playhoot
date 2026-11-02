package games

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// maxInlineSourceBytes is the most a revision's sources may weigh together to
// be written into the context page; past it the page links each file.
const maxInlineSourceBytes = 150 << 10

// sdkDocsURL is the SDK documentation an AI is pointed to. Provisional until
// the frontend's creation guide (game-creation-web-v1 item G) is published.
const sdkDocsURL = "https://playhoot.app/docs/sdk/v1"

// aiContextFormat names the JSON form of the context page.
const aiContextFormat = "playhoot.ai-context.v1"

// notesPath is the project file where the game's agreed rules and pending
// decisions are kept; the page shows it before the sources.
const notesPath = "notes.md"

const codeFileNotFound = "file_not_found"

// aiURLs are the addresses an AI uses through its link.
type aiURLs struct {
	Self        string `json:"self"`
	Changes     string `json:"changes"`
	Restore     string `json:"restore"`
	Build       string `json:"build"`
	Files       string `json:"files"`
	Assets      string `json:"assets"`
	Simulations string `json:"simulations"`
}

type aiContextResponse struct {
	Format        string        `json:"format"`
	GameName      string        `json:"game_name"`
	ObservedAt    time.Time     `json:"observed_at"`
	Revision      int64         `json:"revision"`
	SourcesInline bool          `json:"sources_inline"`
	SDKDocsURL    string        `json:"sdk_docs_url"`
	URLs          aiURLs        `json:"urls"`
	Draft         draftResponse `json:"draft"`
}

// handleAIContext answers the context page of the game an AI's link names:
// everything it needs to propose the next change, as Markdown, or as JSON
// with ?format=json. It reads the current revision; nothing is changed.
func (h *Handler) handleAIContext(w http.ResponseWriter, r *http.Request) {
	userUUID, gameUUID, ok := caller(w, r, "reading AI context")
	if !ok {
		return
	}
	format := r.URL.Query().Get("format")
	if format != "" && format != "json" && format != "markdown" {
		httpx.WriteError(w, http.StatusBadRequest, "format must be json or markdown")
		return
	}
	snapshot, ok := h.loadDraft(w, r, userUUID, gameUUID, 0, true)
	if !ok {
		return
	}
	observedAt := h.now().UTC().Truncate(time.Second)
	urls := aiAddresses(h.links.PublicBaseURL, r.PathValue("token"))
	inline := sourceBytes(snapshot.Files) <= maxInlineSourceBytes
	if !inline {
		for i := range snapshot.Files {
			snapshot.Files[i].Content = nil
		}
	}
	if format == "json" {
		httpx.WriteJSON(w, http.StatusOK, aiContextResponse{
			Format: aiContextFormat, GameName: projectName(snapshot.Files), ObservedAt: observedAt,
			Revision: snapshot.Revision, SourcesInline: inline, SDKDocsURL: sdkDocsURL, URLs: urls, Draft: draftBody(snapshot),
		})
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(aiContextMarkdown(snapshot, urls, observedAt, inline)))
}

// handleAIFile answers one file of a revision (?revision=N, the current one
// by default) as plain text.
func (h *Handler) handleAIFile(w http.ResponseWriter, r *http.Request) {
	userUUID, gameUUID, ok := caller(w, r, "reading AI file")
	if !ok {
		return
	}
	var revision int64
	if raw := r.URL.Query().Get("revision"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			httpx.WriteError(w, http.StatusBadRequest, "revision must be a positive integer")
			return
		}
		revision = parsed
	}
	snapshot, ok := h.loadDraft(w, r, userUUID, gameUUID, revision, true)
	if !ok {
		return
	}
	wanted := r.PathValue("path")
	for _, file := range snapshot.Files {
		if file.Path == wanted && file.Content != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("X-Playhoot-Revision", strconv.FormatInt(snapshot.Revision, 10))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(*file.Content))
			return
		}
	}
	httpx.WriteCodedError(w, http.StatusNotFound, codeFileNotFound, "the revision has no such file")
}

func aiAddresses(base string, token string) aiURLs {
	self := base + aiPathPrefix + token
	return aiURLs{
		Self: self, Changes: self + "/changes", Restore: self + "/restore",
		Build: self + "/build", Files: self + "/files", Assets: self + "/assets", Simulations: self + "/simulations",
	}
}

func sourceBytes(files []sessionpkg.DraftFile) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

// projectName is the name game.project.json gives the game, or "untitled".
func projectName(files []sessionpkg.DraftFile) string {
	for _, file := range files {
		if file.Path != "game.project.json" || file.Content == nil {
			continue
		}
		var project struct {
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(*file.Content), &project) == nil && strings.TrimSpace(project.Name) != "" {
			return strings.TrimSpace(project.Name)
		}
	}
	return "untitled"
}

// aiContextMarkdown writes the context page for an AI reader, with stable
// headings it can rely on.
func aiContextMarkdown(snapshot sessionpkg.DraftSnapshot, urls aiURLs, observedAt time.Time, inline bool) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	line("# Playhoot game: %s", projectName(snapshot.Files))
	line("")
	line("Observed revision **%d** at %s UTC. This page is a read of that revision only: read this link again after any change, never assume what happened.", snapshot.Revision, observedAt.Format(time.RFC3339))
	line("")

	line("## How to change this game")
	line("")
	line("A change is one JSON document. `files` holds only the files that change, each with its complete new content; `delete` names files to remove; `base_revision` is the revision you read (%d); `idempotency_key` is a new random string for each change you send:", snapshot.Revision)
	line("")
	line("```json")
	line(`{"format": "playhoot.project-change.v1", "base_revision": %d, "files": [{"path": "server/game.ts", "content": "<the whole new file>"}], "delete": [], "idempotency_key": "<new random string>"}`, snapshot.Revision)
	line("```")
	line("")
	line("- If you cannot send HTTP requests, give the document to the person: they paste it in the section **Cambios de mi IA** of the game in Playhoot and tell you when it is applied.")
	line("- If you can, POST it to %s . A `revision_conflict` answer means the game changed since you read it: read this link again and redo the change on the new revision.", urls.Changes)
	line("- To go back to an earlier revision, POST `{\"revision\": N, \"base_revision\": %d, \"idempotency_key\": \"...\"}` to %s .", snapshot.Revision, urls.Restore)
	line("- Assets (images, audio, video) are declared in `game.project.json` under `\"assets\"`. The person uploads them in the section **Recursos**, or you PUT the raw file to %s/<key> with an `Idempotency-Key` header.", urls.Assets)
	line("- Every new revision is built automatically. Wait for its result with GET %s?revision=N&wait=20 .", urls.Build)
	line("- To check what the game does, simulate it: POST a scenario `{\"format\": \"playhoot.scenario.v1\", \"revision\": N, \"players\": 2, \"seed\": 1, \"duration_ms\": 30000, \"steps\": [{\"at_ms\": 100, \"player\": \"P1\", \"command\": {\"name\": \"...\", \"payload\": {}}}, {\"at_ms\": 200, \"player\": \"P2\", \"inputs\": {...}}], \"observe_at_ms\": [1000]}` to %s ; the answer has the whole state and every player's view at each observation and at the end, each command's response, refused steps, events and errors. A revision can be simulated once its build SUCCEEDED. If you cannot send HTTP requests, give the scenario to the person: they paste it in the section **Escenarios** and send you the result.", urls.Simulations)
	line("- SDK documentation: %s", sdkDocsURL)
	line("")

	line("## Build of revision %d", snapshot.Revision)
	line("")
	if snapshot.Build == nil {
		line("No build yet.")
	} else {
		line("Status: **%s**", snapshot.Build.Status)
		for _, d := range snapshot.Build.Diagnostics {
			where := d.File
			if where != "" && d.Line > 0 {
				where = fmt.Sprintf("%s:%d:%d", d.File, d.Line, d.Column)
			}
			line("- `%s` [%s] %s", where, d.Stage, d.Message)
		}
	}
	if snapshot.LastSuccessfulBuild != nil {
		line("")
		line("Last successful build: revision %d.", snapshot.LastSuccessfulBuild.Revision)
	} else {
		line("")
		line("No revision has built successfully yet.")
	}
	line("")

	line("## Assets")
	line("")
	if len(snapshot.Assets) == 0 {
		line("No assets declared.")
	}
	for _, asset := range snapshot.Assets {
		required := "required"
		if asset.Optional {
			required = "optional"
		}
		line("- `%s` (%s, %s): **%s** — %s", asset.Key, asset.Type, required, asset.Status, asset.Description)
	}
	for _, d := range snapshot.AssetDiagnostics {
		line("- problem in `%s` (%s): %s", d.Path, d.Code, d.Message)
	}
	line("")

	line("## Recent attempts")
	line("")
	if len(snapshot.Attempts) == 0 {
		line("Nothing was sent yet.")
	}
	for _, attempt := range snapshot.Attempts {
		result := attempt.Outcome
		if attempt.Revision != nil {
			result = fmt.Sprintf("%s → revision %d", attempt.Outcome, *attempt.Revision)
		}
		line("- %s %s on base %d from %s: %s", attempt.CreatedAt.UTC().Format(time.RFC3339), attempt.Kind, attempt.BaseRevision, attempt.Origin, result)
		for _, d := range attempt.Diagnostics {
			line("  - `%s` (%s): %s", d.Path, d.Code, d.Message)
		}
	}
	line("")

	line("## Notes (%s)", notesPath)
	line("")
	notes := false
	for _, file := range snapshot.Files {
		if file.Path == notesPath && file.Content != nil {
			line("%s", strings.TrimRight(*file.Content, "\n"))
			notes = true
		}
	}
	if !notes {
		line("See %s/%s?revision=%d .", urls.Files, notesPath, snapshot.Revision)
	}
	line("")

	line("## Sources of revision %d", snapshot.Revision)
	line("")
	if !inline {
		line("The sources are too large for this page; read each file:")
	}
	for _, file := range snapshot.Files {
		if file.Path == notesPath {
			continue
		}
		if !inline || file.Content == nil {
			line("- `%s` (%d bytes): %s/%s?revision=%d", file.Path, file.Size, urls.Files, file.Path, snapshot.Revision)
			continue
		}
		fence := codeFence(*file.Content)
		line("### `%s`", file.Path)
		line("")
		line("%s%s", fence, codeLanguage(file.Path))
		line("%s", strings.TrimRight(*file.Content, "\n"))
		line("%s", fence)
		line("")
	}
	return b.String()
}

// codeFence is a fence longer than any run of backticks in content, so a
// file holding one never ends its block early.
func codeFence(content string) string {
	longest, run := 0, 0
	for _, c := range content {
		if c == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", max(3, longest+1))
}

func codeLanguage(file string) string {
	switch path.Ext(file) {
	case ".ts":
		return "ts"
	case ".tsx":
		return "tsx"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	default:
		return ""
	}
}
