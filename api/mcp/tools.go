package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/diegobermudez03/playhoot/api/games"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// The change envelope's format (the games operation's own).
const changeFormat = "playhoot.project-change.v1"

// maxAssetBase64 is the longest base64 an upload may carry: the largest
// asset plus the one byte that tells a file over the limit, encoded.
var maxAssetBase64 = base64.StdEncoding.EncodedLen(sessionpkg.MaxDraftAssetBytes + 1)

type listGamesInput struct{}

type createGameInput struct {
	Name           string `json:"name" jsonschema:"the game's name, 1 to 80 characters; the user can change it later"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"a new random string for this creation; resend the same one only to retry it"`
}

type gameInput struct {
	GameUUID string `json:"game_uuid" jsonschema:"the game's uuid, from list_my_games or create_game"`
}

type readFilesInput struct {
	GameUUID string   `json:"game_uuid" jsonschema:"the game's uuid"`
	Paths    []string `json:"paths" jsonschema:"the files' paths in the project, such as server/game.ts; at most 50"`
	Revision int64    `json:"revision,omitempty" jsonschema:"the revision to read; the current one when omitted"`
}

type listRevisionsInput struct {
	GameUUID string `json:"game_uuid" jsonschema:"the game's uuid"`
	Before   int64  `json:"before,omitempty" jsonschema:"list the revisions numbered below this one; from the current one when omitted"`
	Limit    int    `json:"limit,omitempty" jsonschema:"how many revisions, at most 100 (the default)"`
}

type listChangesInput struct {
	GameUUID string `json:"game_uuid" jsonschema:"the game's uuid"`
	Since    int64  `json:"since" jsonschema:"the revision you already know; 0 lists every file as added"`
	Revision int64  `json:"revision,omitempty" jsonschema:"compare up to this revision; the current one when omitted"`
}

type diffRevisionsInput struct {
	GameUUID string `json:"game_uuid" jsonschema:"the game's uuid"`
	From     int64  `json:"from" jsonschema:"the earlier revision; 0 is the empty project"`
	To       int64  `json:"to,omitempty" jsonschema:"the later revision; the current one when omitted"`
	Path     string `json:"path,omitempty" jsonschema:"only this file"`
}

type sdkDocInput struct {
	Page string `json:"page" jsonschema:"a page of the guide, as get_sdk_index lists them: index.md, policies.md, server.md, examples/quiz.md..."`
}

type sdkIndexInput struct{}

type changeFile struct {
	Path    string `json:"path" jsonschema:"the file's path in the project"`
	Content string `json:"content" jsonschema:"the file's complete new content"`
}

type changeEdit struct {
	Path    string `json:"path" jsonschema:"an existing file's path"`
	Find    string `json:"find" jsonschema:"text copied exactly from the file, with enough surrounding lines to appear once"`
	Replace string `json:"replace" jsonschema:"the text that takes its place"`
	All     bool   `json:"all,omitempty" jsonschema:"replace every occurrence of find instead of requiring exactly one"`
}

type applyChangesInput struct {
	GameUUID       string       `json:"game_uuid" jsonschema:"the game's uuid"`
	BaseRevision   int64        `json:"base_revision" jsonschema:"the revision you read; a revision_conflict answer means the game changed since: read it again"`
	Edits          []changeEdit `json:"edits,omitempty" jsonschema:"find/replace edits of existing files, applied in order on base_revision: the default way to change a file"`
	Files          []changeFile `json:"files,omitempty" jsonschema:"new files, or files rewritten substantially, each with its whole new content"`
	Delete         []string     `json:"delete,omitempty" jsonschema:"paths of files to remove"`
	IdempotencyKey string       `json:"idempotency_key" jsonschema:"a new random string for this change; resend the same one only to retry it"`
}

type restoreRevisionInput struct {
	GameUUID       string `json:"game_uuid" jsonschema:"the game's uuid"`
	Revision       int64  `json:"revision" jsonschema:"the earlier revision whose files and assets come back"`
	BaseRevision   int64  `json:"base_revision" jsonschema:"the current revision you read"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"a new random string for this restore"`
}

type uploadAssetInput struct {
	GameUUID       string `json:"game_uuid" jsonschema:"the game's uuid"`
	Key            string `json:"key" jsonschema:"the asset's key, as game.project.json declares it"`
	ContentBase64  string `json:"content_base64" jsonschema:"the file's bytes in standard base64; its real type must match the declared one (image, audio or video) and stay within its size limit"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"a new random string for this upload"`
}

type removeAssetInput struct {
	GameUUID       string `json:"game_uuid" jsonschema:"the game's uuid"`
	Key            string `json:"key" jsonschema:"the asset's key"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"a new random string for this removal"`
}

type getBuildInput struct {
	GameUUID    string `json:"game_uuid" jsonschema:"the game's uuid"`
	Revision    int64  `json:"revision" jsonschema:"the revision whose build to read"`
	WaitSeconds int    `json:"wait_seconds,omitempty" jsonschema:"wait up to this many seconds (at most 20) for the build to end"`
}

type simulateInput struct {
	GameUUID     string         `json:"game_uuid" jsonschema:"the game's uuid"`
	Scenario     map[string]any `json:"scenario,omitempty" jsonschema:"a playhoot.scenario.v1 document, as the guide's scenarios page describes"`
	ScenarioPath string         `json:"scenario_path,omitempty" jsonschema:"run the scenario saved at this path (scenarios/<name>.json) of the revision instead"`
	All          bool           `json:"all,omitempty" jsonschema:"run every saved scenario (at most 10), a summary each"`
	Revision     int64          `json:"revision,omitempty" jsonschema:"the revision to simulate; the current one when omitted"`
	Detail       string         `json:"detail,omitempty" jsonschema:"summary (default) or full: every state, view, command answer and event"`
}

type readSimulationStateInput struct {
	GameUUID string `json:"game_uuid" jsonschema:"the game's uuid"`
	ResultID string `json:"result_id" jsonschema:"the result_id a simulate answer gave"`
	AtMs     *int64 `json:"at_ms,omitempty" jsonschema:"an instant the simulation observed (observed_at_ms); the end when omitted"`
	Of       string `json:"of,omitempty" jsonschema:"global (default, the authoritative state) or a player (P1, P2...) for their view"`
	Path     string `json:"path,omitempty" jsonschema:"a path into it, as players.P1.hand[0]; the whole value when omitted"`
}

// addTools declares every tool on server. Each one translates its arguments
// to its games operation, and the operation's answer to its result.
func (h *Handler) addTools(server *sdk.Server) {
	sdk.AddTool(server, &sdk.Tool{Name: "list_my_games", Description: "Lists the user's games, the most recently changed first, with whether each is published and has a version being edited."},
		func(ctx context.Context, req *sdk.CallToolRequest, _ listGamesInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.ListGames(ctx, user))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "create_game", Description: "Creates a new, unpublished game for the user, with a starter project as revision 1 (it builds automatically). Answers its game_uuid."},
		func(ctx context.Context, req *sdk.CallToolRequest, in createGameInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.CreateGame(ctx, user, games.CreateGameRequest{Name: in.Name, IdempotencyKey: in.IdempotencyKey}))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "get_game_context", Description: "Reads where a game stands: its current revision and when it was observed, every file's path, hash and size (not their contents: read those with read_files), the build of that revision (status and its first diagnostics), the last revision that built, the declared assets with their status, the recent attempts, notes.md and the SDK guide's address."},
		func(ctx context.Context, req *sdk.CallToolRequest, in gameInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			// No link: the tools replace the link's addresses.
			read, refusal := h.games.ReadContext(ctx, user, in.GameUUID, "", false)
			return answer(read.Response, refusal)
		})
	sdk.AddTool(server, &sdk.Tool{Name: "read_files", Description: "Reads the files you name of a revision of the game, with their contents and hashes; missing lists the paths the revision does not have. Read only the files you need."},
		func(ctx context.Context, req *sdk.CallToolRequest, in readFilesInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.ReadFiles(ctx, user, in.GameUUID, in.Paths, optionalRevision(in.Revision)))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "list_changes_since", Description: "Lists what changed in the game since a revision you already know: the paths added, modified and deleted, and every file's hash and size at the current revision (or the one given). No contents: read the changed files you need with read_files."},
		func(ctx context.Context, req *sdk.CallToolRequest, in listChangesInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.ReadChanges(ctx, user, in.GameUUID, in.Since, optionalRevision(in.Revision)))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "apply_changes", Description: "Changes the game's sources on the revision you read, all or nothing. Change existing files with edits (find/replace, applied in order; find must appear exactly once unless all is set, otherwise edit_not_found or edit_ambiguous names the edit and nothing applies); use files only for new files or substantial rewrites; delete removes paths. A path goes in one of the three only. Applied, it makes a new revision, which builds automatically (wait for it with get_build); the answer has every file's hash, what changed and the diff of each edited file."},
		func(ctx context.Context, req *sdk.CallToolRequest, in applyChangesInput) (*sdk.CallToolResult, any, error) {
			by, err := callerOf(req)
			if err != nil {
				return nil, nil, err
			}
			change := games.ChangeRequest{Format: changeFormat, BaseRevision: &in.BaseRevision, Delete: in.Delete, IdempotencyKey: in.IdempotencyKey}
			if change.Delete == nil {
				change.Delete = []string{}
			}
			for _, file := range in.Files {
				change.Files = append(change.Files, games.ChangeFile{Path: file.Path, Content: file.Content})
			}
			for _, edit := range in.Edits {
				change.Edits = append(change.Edits, games.ChangeEdit{Path: edit.Path, Find: edit.Find, Replace: edit.Replace, All: edit.All})
			}
			return answer(h.games.ApplyChanges(ctx, by, in.GameUUID, change))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "restore_revision", Description: "Makes a new revision with an earlier revision's files and assets, to recover from a broken change. History is never rewritten."},
		func(ctx context.Context, req *sdk.CallToolRequest, in restoreRevisionInput) (*sdk.CallToolResult, any, error) {
			by, err := callerOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.RestoreRevision(ctx, by, in.GameUUID, games.RestoreRequest{Revision: in.Revision, BaseRevision: &in.BaseRevision, IdempotencyKey: in.IdempotencyKey}))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "list_revisions", Description: "Lists the game's revisions, newest first, with their provenance: kind (template, edition, change, restore, asset), origin (web, api, mcp; none when the platform made it), actor (person, ai, platform), parent, restored_from, from (the version it was copied from) and sdk; a page of at most limit, below before."},
		func(ctx context.Context, req *sdk.CallToolRequest, in listRevisionsInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.ListRevisions(ctx, user, in.GameUUID, in.Before, in.Limit))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "diff_revisions", Description: "Compares two revisions of the game: each file added, modified or deleted with its unified diff (8 KiB per file, 64 KiB in all; a file marked omitted is listed without its diff, ask for its path alone). Use it to review what a change or a restore did, or to recover a good part of an earlier revision."},
		func(ctx context.Context, req *sdk.CallToolRequest, in diffRevisionsInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.ReadDiff(ctx, user, in.GameUUID, in.From, optionalRevision(in.To), in.Path))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "get_sdk_index", Description: "The SDK guide's compact index: what the SDK gives, one line each, with its page and an example, the SDK version, and what it does not provide. Read it first, then only the pages you need with get_sdk_doc. The guide belongs to the toolchain your game builds with."},
		func(ctx context.Context, req *sdk.CallToolRequest, _ sdkIndexInput) (*sdk.CallToolResult, any, error) {
			if _, err := userOf(req); err != nil {
				return nil, nil, err
			}
			return markdown(h.games.ReadSDKPage(ctx, sessionpkg.SDKGuideIndex))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "get_sdk_doc", Description: "One page of the SDK guide, in Markdown: index.md (start here), policies.md, project.md, server.md, client.md, ui.md, phases.md, assets.md, changes.md, building.md, scenarios.md, or an example (examples/<name>.md)."},
		func(ctx context.Context, req *sdk.CallToolRequest, in sdkDocInput) (*sdk.CallToolResult, any, error) {
			if _, err := userOf(req); err != nil {
				return nil, nil, err
			}
			return markdown(h.games.ReadSDKPage(ctx, in.Page))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "upload_asset", Description: "Uploads the file of an asset game.project.json declares, in base64. Limits by type: image 5 MiB, audio 10 MiB, video 50 MiB, 100 MiB in all. If you cannot produce the file, ask the user to upload it in Recursos."},
		func(ctx context.Context, req *sdk.CallToolRequest, in uploadAssetInput) (*sdk.CallToolResult, any, error) {
			by, err := callerOf(req)
			if err != nil {
				return nil, nil, err
			}
			if len(in.ContentBase64) > maxAssetBase64 {
				return refusal("asset_too_large", fmt.Sprintf("the file is larger than the largest asset allowed (%d bytes)", sessionpkg.MaxDraftAssetBytes)), nil, nil
			}
			content, err := base64.StdEncoding.DecodeString(in.ContentBase64)
			if err != nil {
				return refusal("invalid_base64", "content_base64 must be standard base64"), nil, nil
			}
			return answer(h.games.UploadAsset(ctx, by, in.GameUUID, in.Key, in.IdempotencyKey, bytes.NewReader(content)))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "remove_asset", Description: "Removes the file bound to an asset key in a new revision."},
		func(ctx context.Context, req *sdk.CallToolRequest, in removeAssetInput) (*sdk.CallToolResult, any, error) {
			by, err := callerOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.RemoveAsset(ctx, by, in.GameUUID, in.Key, in.IdempotencyKey))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "get_build", Description: "Reads the build of a revision: QUEUED, RUNNING, SUCCEEDED, FAILED or SUPERSEDED, with diagnostics by file and line. With wait_seconds it answers as soon as the build ends."},
		func(ctx context.Context, req *sdk.CallToolRequest, in getBuildInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			var wait time.Duration
			if in.WaitSeconds > 0 {
				var refusal *games.Refusal
				if wait, refusal = games.BuildWait(in.WaitSeconds); refusal != nil {
					return refused(refusal), nil, nil
				}
			}
			return answer(h.games.ReadBuild(ctx, user, in.GameUUID, &in.Revision, wait))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "simulate", Description: "Runs a scenario on a revision's succeeded build with a virtual clock (same seed, same result): the scenario given, the one saved at scenario_path, or all saved ones. Expectations (expect) are checked by the platform; holds keep inputs pressed for a while. The answer is a summary: passed, each failed expectation with expected and actual, how it stopped, counts, the first refusals and errors, and a result_id to read any observed state with read_simulation_state. detail full answers everything instead."},
		func(ctx context.Context, req *sdk.CallToolRequest, in simulateInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			simulation := games.SimulateRequest{Revision: in.Revision, Detail: in.Detail, ScenarioPath: in.ScenarioPath, All: in.All}
			if in.ScenarioPath == "" && !in.All {
				simulation.Scenario = mustJSON(in.Scenario)
			}
			return answer(h.games.Simulate(ctx, user, in.GameUUID, simulation))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "read_simulation_state", Description: "Reads one value of a simulation result while it is kept (30 minutes): the global state or a player's view, at an observed instant or the end, whole or at a path. A value over 64 KiB answers its size and keys instead: ask for a narrower path."},
		func(ctx context.Context, req *sdk.CallToolRequest, in readSimulationStateInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.ReadSimulationState(ctx, user, in.GameUUID, in.ResultID, in.AtMs, in.Of, in.Path))
		})
}

// optionalRevision is a revision a tool names, nil when it names none (0).
func optionalRevision(revision int64) *int64 {
	if revision == 0 {
		return nil
	}
	return &revision
}

// callerOf is the User a tool call acts for, changing as an AI application
// connected through MCP.
func callerOf(req *sdk.CallToolRequest) (games.Caller, error) {
	user, err := userOf(req)
	if err != nil {
		return games.Caller{}, err
	}
	return games.Caller{UserUUID: user, Origin: sessionpkg.DraftOriginMCP}, nil
}

// markdown is a tool's result for a guide page: its Markdown as is (no JSON
// escaping), after a line naming the toolchain; or the refusal.
func markdown(page *games.SDKPageResponse, refusal *games.Refusal) (*sdk.CallToolResult, any, error) {
	if refusal != nil {
		return refused(refusal), nil, nil
	}
	content := page.Content
	if page.ToolchainVersion != "" {
		content = "<!-- " + page.Page + ", toolchain " + page.ToolchainVersion + " -->\n" + content
	}
	return text(content), nil, nil
}

// answer is a tool's result for an operation's answer: its value as JSON, or
// an error result with the status and body its HTTP route answers with.
func answer[T any](value T, refusal *games.Refusal) (*sdk.CallToolResult, any, error) {
	if refusal != nil {
		return refused(refusal), nil, nil
	}
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(value); err != nil {
		return nil, nil, fmt.Errorf("encoding the tool's answer: %w", err)
	}
	return text(encoded.String()), nil, nil
}

// refused is the error result of a refusal: {status, answer}, answer being
// the body the HTTP route answers.
func refused(refusal *games.Refusal) *sdk.CallToolResult {
	var body any
	if json.Unmarshal(mustJSON(refusal.Body), &body) != nil {
		body = refusal.Body
	}
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: string(mustJSON(map[string]any{"status": refusal.Status, "answer": body}))}}}
}

func text(content string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: content}}}
}

// refusal is an error result decided before any operation ran.
func refusal(code string, message string) *sdk.CallToolResult {
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: string(mustJSON(map[string]any{"code": code, "message": message}))}}}
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte("null")
	}
	return encoded
}
