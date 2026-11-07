package games

import (
	"context"
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

// maxContextDiagnostics is how many of a build's diagnostics a context
// carries; the build read has them all.
const maxContextDiagnostics = 20

// guidePath is where the web app serves the SDK guide for AI agents, below
// the web's public address (the frontend's apps/web/public/docs/sdk/v1).
const guidePath = "/docs/sdk/v1"

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
	Changed     string `json:"changed"`
	Revisions   string `json:"revisions"`
	Diff        string `json:"diff"`
	SDK         string `json:"sdk"`
	Restore     string `json:"restore"`
	Build       string `json:"build"`
	Files       string `json:"files"`
	Assets      string `json:"assets"`
	Simulations string `json:"simulations"`
}

// AIContextResponse is the JSON form of a game's context: what an AI needs
// to know where the game stands, never the sources unless SourcesInline (the
// Markdown page only). Notes is the content of notes.md; Brief brief.json
// as written and Phase the work's phase it records.
type AIContextResponse struct {
	Format        string          `json:"format"`
	GameName      string          `json:"game_name"`
	ObservedAt    time.Time       `json:"observed_at"`
	Revision      int64           `json:"revision"`
	SourcesInline bool            `json:"sources_inline"`
	Notes         *string         `json:"notes,omitempty"`
	Brief         json.RawMessage `json:"brief,omitempty"`
	Phase         string          `json:"phase,omitempty"`
	SDKDocsURL    string          `json:"sdk_docs_url"`
	URLs          *aiURLs         `json:"urls,omitempty"`
	Draft         draftResponse   `json:"draft"`
}

// AIContext is everything an AI needs to propose the next change to a game:
// Response, its JSON form, and what its Markdown page is written from.
type AIContext struct {
	Response AIContextResponse

	name     string
	snapshot sessionpkg.DraftSnapshot
}

// handleAIContext answers the context page of the game an AI's link names:
// everything it needs to propose the next change, as Markdown with the
// sources inline when small (a chat that only reads links gets everything in
// one read), or as JSON without them with ?format=json. It reads the current
// revision; nothing is changed.
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
	read, refusal := h.ReadContext(r.Context(), userUUID, gameUUID, r.PathValue("token"), format != "json")
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	// Without a link (the owner's own route) there are no addresses to write
	// a page with: the JSON form answers.
	if format == "json" || read.Response.URLs == nil {
		httpx.WriteJSON(w, http.StatusOK, read.Response)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(aiContextMarkdown(read.name, read.snapshot, *read.Response.URLs, read.Response.URLs.SDK, read.Response.ObservedAt, read.Response.SourcesInline, read.Response.Notes, read.Response.Brief, read.Response.Phase)))
}

// ReadContext reads the context of a game for an AI, at its current
// revision. linkToken is the AI link it is read through, empty through MCP,
// whose tools replace the link's addresses. Only with inlineSources (the
// Markdown page) does it carry the sources' contents, when small; otherwise
// it carries their paths, hashes and sizes, and notes.md.
func (h *Handler) ReadContext(ctx context.Context, userUUID string, rawGameUUID string, linkToken string, inlineSources bool) (AIContext, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return AIContext{}, refusal
	}
	snapshot, name, refusal := h.loadDraft(ctx, userUUID, gameUUID, 0, true)
	if refusal != nil {
		return AIContext{}, refusal
	}
	observedAt := h.now().UTC().Truncate(time.Second)
	var urls *aiURLs
	if linkToken != "" {
		addresses := aiAddresses(h.links.PublicBaseURL, linkToken)
		urls = &addresses
	}
	var notes *string
	var brief json.RawMessage
	var phase string
	for _, file := range snapshot.Files {
		if file.Path == notesPath && file.Content != nil {
			notes = file.Content
		}
		// A stored brief is valid: it was checked when written.
		if file.Path == sessionpkg.BriefPath && file.Content != nil {
			if recorded, valid := sessionpkg.BriefPhase(*file.Content); valid {
				brief, phase = json.RawMessage(*file.Content), recorded
			}
		}
	}
	inline := inlineSources && sourceBytes(snapshot.Files) <= maxInlineSourceBytes
	if !inline {
		for i := range snapshot.Files {
			snapshot.Files[i].Content = nil
		}
	}
	draft := draftBody(snapshot)
	draft.Build = firstDiagnostics(draft.Build)
	// Through a link, the guide of the toolchain this game builds with; the
	// web app's copy otherwise.
	sdkDocs := h.links.GuideBaseURL + "/index.md"
	if urls != nil {
		sdkDocs = urls.SDK + "/index.md"
	}
	return AIContext{
		Response: AIContextResponse{
			Format: aiContextFormat, GameName: gameName(name), ObservedAt: observedAt,
			Revision: snapshot.Revision, SourcesInline: inline, Notes: notes, Brief: brief, Phase: phase, SDKDocsURL: sdkDocs, URLs: urls, Draft: draft,
		},
		name:     gameName(name),
		snapshot: snapshot,
	}, nil
}

// handleAIFile answers one file of a revision (?revision=N, the current one
// by default) as plain text.
func (h *Handler) handleAIFile(w http.ResponseWriter, r *http.Request) {
	userUUID, gameUUID, ok := caller(w, r, "reading AI file")
	if !ok {
		return
	}
	revision, refusal := revisionQuery(r.URL.Query().Get("revision"))
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	file, refusal := h.ReadFile(r.Context(), userUUID, gameUUID, r.PathValue("path"), revision)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Playhoot-Revision", strconv.FormatInt(file.Revision, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(file.Content))
}

// File is one file of a revision of the version being built.
type File struct {
	Content  string
	Revision int64
}

// ReadFile reads the file at path of a revision (nil: the current one).
func (h *Handler) ReadFile(ctx context.Context, userUUID string, rawGameUUID string, path string, revision *int64) (File, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return File{}, refusal
	}
	if refusal := revisionRefusal(revision); refusal != nil {
		return File{}, refusal
	}
	var number int64
	if revision != nil {
		number = *revision
	}
	snapshot, _, refusal := h.loadDraft(ctx, userUUID, gameUUID, number, true)
	if refusal != nil {
		return File{}, refusal
	}
	for _, file := range snapshot.Files {
		if file.Path == path && file.Content != nil {
			return File{Content: *file.Content, Revision: snapshot.Revision}, nil
		}
	}
	return File{}, refuseCoded(http.StatusNotFound, codeFileNotFound, "the revision has no such file")
}

// firstDiagnostics is build with only its first diagnostics, the count
// telling how many it has.
func firstDiagnostics(build *BuildResponse) *BuildResponse {
	if build == nil || len(build.Diagnostics) <= maxContextDiagnostics {
		return build
	}
	trimmed := *build
	trimmed.Diagnostics = build.Diagnostics[:maxContextDiagnostics]
	return &trimmed
}

func aiAddresses(base string, token string) aiURLs {
	self := base + aiPathPrefix + token
	return aiURLs{
		Self: self, Changes: self + "/changes", Changed: self + "/changed", Revisions: self + "/revisions", Diff: self + "/diff", SDK: self + "/sdk", Restore: self + "/restore",
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

// gameName is the name on the game's card, or "untitled".
func gameName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "untitled"
	}
	return name
}

// aiContextMarkdown writes the context page for an AI reader, with stable
// headings it can rely on.
func aiContextMarkdown(name string, snapshot sessionpkg.DraftSnapshot, urls aiURLs, guide string, observedAt time.Time, inline bool, notes *string, brief json.RawMessage, phase string) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	line("# Playhoot game: %s", name)
	line("")
	line("Observed revision **%d** at %s UTC. This page is a read of that revision only: read this link again after any change, never assume what happened.", snapshot.Revision, observedAt.Format(time.RFC3339))
	line("")

	line("## How to change this game")
	line("")
	line("A change is one JSON document. `edits` change existing files: each replaces `find`, text copied exactly from the file that appears in it once (or every occurrence with `\"all\": true`), with `replace`, in order. `files` holds new files, or files rewritten substantially, each with its complete content; `delete` names files to remove. A path goes in one of the three only. `base_revision` is the revision you read (%d); `idempotency_key` is a new random string for each change you send:", snapshot.Revision)
	line("")
	line("```json")
	line(`{"format": "playhoot.project-change.v1", "base_revision": %d, "edits": [{"path": "server/game.ts", "find": "const rounds = 5;", "replace": "const rounds = 3;"}], "files": [], "delete": [], "idempotency_key": "<new random string>"}`, snapshot.Revision)
	line("```")
	line("")
	line("- Prefer edits: they are small and cheap. If an edit's `find` is not in the file (`edit_not_found`) or appears more than once (`edit_ambiguous`), nothing applies and the diagnostic names the edit by its index: read the file again and fix that edit.")
	line("- If you cannot send HTTP requests, give the document to the person: they paste it in the section **Cambios de mi IA** of the game in Playhoot and tell you when it is applied.")
	line("- Read only what you need: one file as text at %s/<path>?revision=N, several as JSON at %s?path=<path>&path=<path>, and what changed since a revision you already know (paths and hashes, no contents) at %s?since=N . The revisions, newest first, with where each came from: %s . What changed between two revisions, as unified diffs: %s?from=N&to=M (to defaults to the current one; add &path=<path> for one file).", urls.Files, urls.Files, urls.Changed, urls.Revisions, urls.Diff)
	line("- If you can, POST it to %s . The answer has the new revision with every file's hash and what changed. A `revision_conflict` answer means the game changed since you read it: read what changed since your revision and redo the change on the new one.", urls.Changes)
	line("- To go back to an earlier revision, POST `{\"revision\": N, \"base_revision\": %d, \"idempotency_key\": \"...\"}` to %s .", snapshot.Revision, urls.Restore)
	line("- Assets (images, audio, video) are declared in `game.project.json` under `\"assets\"`. The person uploads them in the section **Recursos**, or you PUT the raw file to %s/<key> with an `Idempotency-Key` header.", urls.Assets)
	line("- Every new revision is built automatically. Wait for its result with GET %s?revision=N&wait=20 .", urls.Build)
	line("- To check what the game does, simulate it: POST a scenario `{\"format\": \"playhoot.scenario.v1\", \"revision\": N, \"players\": 2, \"seed\": 1, \"duration_ms\": 30000, \"steps\": [{\"at_ms\": 100, \"player\": \"P1\", \"command\": {\"name\": \"...\", \"payload\": {}}}], \"holds\": [{\"from_ms\": 1000, \"to_ms\": 1800, \"player\": \"P2\", \"inputs\": {...}}], \"expect\": [{\"at_ms\": 2000, \"of\": \"global\", \"path\": \"scores.P1\", \"op\": \"eq\", \"value\": 1}]}` to %s (or `{\"scenario_path\": \"scenarios/<name>.json\"}` / `{\"all\": true}` for the saved ones). The answer is a summary: passed, each failed expectation with expected and actual, how it stopped, counts and a `result_id`; read any observed state at %s/<result_id>/state?at_ms=N&of=P1&path=a.b (`\"detail\": \"full\"` answers everything at once). A revision can be simulated once its build SUCCEEDED. If you cannot send HTTP requests, give the scenario to the person: they paste it in the section **Escenarios** and send you the result.", urls.Simulations, urls.Simulations)
	line("- How to write a Playhoot game (read it before writing code): %s/index.md , then the one-line index of what the SDK gives, %s/capabilities.md , and only the pages your game needs. The order of work and what done means: %s/policies.md ; the change format: %s/changes.md ; scenarios: %s/scenarios.md .", guide, guide, guide, guide, guide)
	line("")

	line("## Build of revision %d", snapshot.Revision)
	line("")
	if snapshot.Build == nil {
		line("No build yet.")
	} else {
		line("Status: **%s**", snapshot.Build.Status)
		shown := snapshot.Build.Diagnostics
		if len(shown) > maxContextDiagnostics {
			shown = shown[:maxContextDiagnostics]
		}
		for _, d := range shown {
			where := d.File
			if where != "" && d.Line > 0 {
				where = fmt.Sprintf("%s:%d:%d", d.File, d.Line, d.Column)
			}
			line("- `%s` [%s] %s", where, d.Stage, d.Message)
		}
		if hidden := len(snapshot.Build.Diagnostics) - len(shown); hidden > 0 {
			line("- and %d more: GET %s?revision=%d", hidden, urls.Build, snapshot.Revision)
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

	line("## Brief (%s)", sessionpkg.BriefPath)
	line("")
	if brief != nil {
		line("Phase of the work: **%s** (brief, foundation, building, testing, assets, ready). Keep it current as the work moves on.", phase)
		line("")
		line("```json")
		line("%s", strings.TrimRight(string(brief), "\n"))
		line("```")
	} else {
		line("This revision has no %s: agree the game with the person and write it first (%s/brief.md).", sessionpkg.BriefPath, guide)
	}
	line("")

	line("## Notes (%s)", notesPath)
	line("")
	if notes != nil {
		line("%s", strings.TrimRight(*notes, "\n"))
	} else {
		line("This revision has no %s.", notesPath)
	}
	line("")

	line("## Sources of revision %d", snapshot.Revision)
	line("")
	if !inline {
		line("The sources are too large for this page; read each file:")
	}
	for _, file := range snapshot.Files {
		if file.Path == notesPath || file.Path == sessionpkg.BriefPath {
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
