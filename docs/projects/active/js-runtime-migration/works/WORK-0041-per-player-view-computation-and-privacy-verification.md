# WORK-0041: Per-Player View Computation & Privacy Verification

Status: IMPLEMENTING
Created: 2026-09-27
Last status change: 2026-09-29 (DRAFT -> READY -> IMPLEMENTING the same day, per explicit human authorization ("proceed") immediately following both Material Decisions' resolution - implementation complete, pending independent review. See Scope Narrowing and Completion Record below.)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`

Canonical context:
- `game/language/v1/program/README.md` (the retired `Projection`/`View`/`Presentation` model, for precedent on what a privacy boundary must cover)
- `docs/projects/active/session-runtime-v1/works/WORK-0015-disconnect-reconnect-full-resync.md` (owns the live reconnect/resync capability this WORK's "retrieve on reconnect" phrasing borders - see Coordination Flag below)

## Outcome

Because authored JavaScript computes per-player views directly (there is no longer a Playhoot-trusted, compiler-enforced `Projection` guaranteeing purity), build the mechanism that ensures no player's `ClientState` can leak another player's or role's private data — primarily by never handing the untrusted script that data in the first place, not by trusting it and checking afterward — and the mechanism to retrieve a player's current `ClientState` on initial load or reconnect directly from persisted state (`WORK-0038`), without requiring every prior transient `SEND_EVENT` to have been received. `ADR-0015` names this a required capability, not optional hardening: knowing all game state does not, by itself, guarantee an authored view is correct.

**Coordination Flag (2026-09-29): `session-runtime-v1`'s `WORK-0015`.** This WORK's own "on initial load or reconnect" phrasing is a pure backend capability only: given a persisted state and a viewer's identity/role, compute that viewer's privacy-safe `ClientState`. It does not cover *when* that computation is invoked over a live connection, debounce/grace timing, or how a reconnecting client's transport session is resumed - that is `session-runtime-v1`'s own `WORK-0015` (Disconnect/Reconnect/Full Resync), still PLANNED, which owns resync over the live-connection layer `WORK-0020` rebuilds. `WORK-0015` should call this WORK's own filter-then-project computation as its own projection step, not reimplement privacy filtering itself; this WORK should not grow a delivery/connection-handling mechanism of its own. Flagged here and in both Projects' own `PROJECT.md`, not silently resolved in either - see `js-runtime-migration/PROJECT.md`'s own Coordination note.

## Context

**Vocabulary update (2026-09-28):** `WORK-0039`'s revised design fixes the mechanism this WORK verifies: the backend script exposes a second, pure entry point, `project(state, viewer, context) -> ClientState`, separate from `execute` — `project` cannot mutate authoritative state and cannot emit Commands (`WORK-0039`'s own Constraints). This WORK's privacy-verification mechanism operates specifically on `project`'s output for a given `viewer`, not on anything returned from `execute`'s Commands (`REQUEST_VIEW` as a command no longer exists). `ClientState`/`PlayerProjection` is the correct term for this WORK's own subject matter — never "View," since it is authorized game data, not a UI instruction, and must never itself be assumed to contain presentation concepts.

**Drafting checked the actual current code rather than trusting the vocabulary decision alone (2026-09-29).** `project`/`ClientState` exist only as prose today — a repository-wide check found zero `.go`/`.proto` references anywhere:

- `session/jsexecutor/proto/executor.proto` declares exactly one RPC, `Execute`. `session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md` documents exactly one script entry point, `execute(previousState, event, context)`. Neither predates nor reflects `WORK-0039`'s 2026-09-28 vocabulary decision.
- `session/internal/executor.Executor` (the Session-side port, `WORK-0053`, DONE) has exactly one method, `Execute(ctx, ExecutionInput) (ExecutionOutput, error)` — no viewer field, no `ClientState` type.
- `session/workflows/sessionlifecycle/internal/platform/LOGICAL_CONTRACT.md`'s own "`execute` vs. `project`" section states the vocabulary but explicitly disclaims ownership: "This package does not own invoking `project` or verifying its privacy properties."
- No other current WORK (checked `PROJECT.md`'s own Capability Coverage table) owns wiring `project` end to end — this Project's single "per-player private views... with privacy verification" row names only this WORK.

**Conclusion: before any privacy-verification mechanism can operate on real output, `project` must actually become a callable entry point** — through the sandbox's script contract, the Executor's gRPC surface, the Session-side port, and a real call site. This is a materially larger scope than "verify an existing mechanism" and is flagged as Material Decision 1 below rather than silently assumed.

**Persisted state this WORK reads (`WORK-0038`, DONE, confirmed against the actual migrations/repo code, not the design doc's paraphrase):** a session's authoritative current state is `session_runtime_turns.new_state` (`JSONB`), the row `sessions.current_turn_id` points at (`session/workflows/sessionlifecycle/internal/repo/runtime_turn.go`'s `GetRuntimeTurn`/`RuntimeTurn.NewState json.RawMessage`). This is exactly `project`'s `state` argument — no second table/snapshot concept is needed to source it, consistent with `WORK-0038`'s own snapshot-based design.

**Viewer identity already available:** `platform.ActorRef` (`session/workflows/sessionlifecycle/internal/platform/event.go`), resolved via `internal/repo.FindActor` and `actorRefForActorID` — the same identity already threaded through every `execute` call site (for example `step_submit_player_event.go`). Host vs. participant is distinguishable today via `sessions.host_actor_id`. A finer ADMIN/PARTICIPANT connection-role split is accepted at the design level (`session/docs/decisions/SESSION-ADR-0024-role-aware-live-connections.md`) but not yet implemented (owned by `session-runtime-v1`'s `WORK-0020`/`WORK-0021`) — this WORK's own `viewer` parameter only needs actor identity plus the host/participant distinction already available today, not that finer split.

**No declared privacy schema exists anywhere.** `session/docs/GAME_VERSION_ARTIFACT_MODEL.md`'s `GameContract` field is explicitly opaque from the artifact model's own perspective and owned by `WORK-0039` (DONE) — it defines no per-field visibility/ownership concept. There is no existing mechanism, anywhere in the artifact model or platform vocabulary, an author could use to declare "this piece of state is private to viewer X." A verification mechanism needs *some* ground truth for what "private" means.

**Architectural resolution (2026-09-29), by explicit human decision — see Material Decisions below.** The privacy boundary is primarily **capability-based, not detection-based**: `project` must never receive state data the invoking viewer is not authorized to see in the first place, rather than being trusted with everything and checked afterward. Leak-detection checks (structural containment, differential/causal probing) remain as defense-in-depth against a filtering bug or a mis-authored schema — they are not the primary guarantee and must not be relied on to compensate for handing the untrusted script unrestricted state access. This changes `project`'s own signature: it receives a Playhoot-constructed, viewer-scoped `ProjectionInput`, never the raw authoritative `state`.

## Scope

### In Scope

- A declared **visibility schema** (addendum to `WORK-0044`'s artifact model, mirroring the `ParticipantConstraints` precedent), distinguishing at least three classes for a given state path: **public/shared** (every viewer's `ProjectionInput`), **private to a participant/owner** (only that viewer's own `ProjectionInput`), and **server-only / unavailable to projection** (never included in any `ProjectionInput`, for any viewer).
- An explicit **declassification mechanism**: an author who needs an authorized derivative of private data exposed to other viewers (for example, an opponent's card *count* without their actual hand) declares that derivative explicitly in the schema. Playhoot itself computes the declared derivative (from a small, fixed, Playhoot-trusted set of primitives — see Approved Design), never the untrusted script, and includes only the computed derivative — never the underlying private value — in other viewers' `ProjectionInput`.
- A Playhoot-side **visibility-filtering step**: given authoritative `state`, a viewer's identity, and the declared schema, construct a viewer-scoped `ProjectionInput` containing only public/shared data, that viewer's own private data, and any declared declassified derivatives. This is the primary privacy guarantee — capability-based, not leak-detection-based.
- Wiring the revised `project(projectionInput, viewer, context) -> ClientState` as a real, callable entry point end to end: sandbox script contract (a second pure global function, reusing `WORK-0035`'s existing worker/isolation infrastructure — not a new deployment or topology), a second RPC on the already-deployed Executor service (`session/jsexecutor`), a second method on the Session-side `Executor` port (`session/internal/executor`), and a real Session Runtime call site that resolves the viewer's `ActorRef`, loads current persisted state, runs the filtering step above, and only then calls the Executor.
- **Defense-in-depth checks**, secondary to the filtering step above, never a substitute for it: (a) a structural containment check (live, cheap) confirming no computed `ClientState` contains a value equal to another viewer's private/server-only data — a safety net against a filtering-step implementation bug; (b) a differential/causal probe (offline, at game-version publish/validation time) that flags an output difference between viewers only when that difference is **not** explained by an already-declared public/declassified dependency — a safety net against a schema-authoring mistake (an author mismarking something as public/declassified that shouldn't be).
- Retrieving a player's current `ClientState` on initial load/reconnect directly from `WORK-0038`'s persisted state (`GetRuntimeTurn` via `sessions.current_turn_id`) plus the requesting viewer's resolved identity, through the same filter-then-project path above — no replay of prior turns/events required.
- A new package-local contract document recording the visibility/declassification model, the filtering step, both defense-in-depth checks, and their respective limits.
- An adversarial-authored-script test proving (1) a script simply cannot access another viewer's excluded data (it is absent from `ProjectionInput`, not merely discouraged), and (2) a deliberately wrong schema (over-broad public declaration) is still caught by the defense-in-depth layer.

### Out of Scope

- *When* a `ClientState` is computed/delivered over a live connection, debounce/grace timing, or resuming a reconnecting client's transport session — `session-runtime-v1`'s own `WORK-0015` (Disconnect/Reconnect/Full Resync), per this WORK's own Coordination Flag above. This WORK exposes a pure `project(projectionInput, viewer, context) -> ClientState` computation (behind its own filtering step) for `WORK-0015` to call as its own projection step; it does not grow a delivery/connection-handling mechanism of its own.
- `SEND_EVENT` delivery — `WORK-0042`'s own scope.
- The finer ADMIN/PARTICIPANT connection-role split (`SESSION-ADR-0024`) — this WORK's `viewer` only needs actor identity plus the already-implemented host/participant distinction.
- An open-ended/extensible declassification primitive vocabulary. A minimal starter set (see Approved Design) is in scope; expanding it later, if a game needs a derivative the starter set cannot express, is a candidate for future WORK, not something this WORK must anticipate exhaustively.
- Formal information-flow proof covering every conceivable derivation. The filtering step removes the *ability* to access excluded data at all (the strongest practical guarantee at this layer); the defense-in-depth checks catch common implementation/schema mistakes, not a fully adversarial author actively working around a correctly-scoped `ProjectionInput` using only what it legitimately contains.

## Approved Design

**Architecture:**

```text
authoritative state (WORK-0038's session_runtime_turns.new_state)
  -> Playhoot visibility filtering (this WORK, using the declared schema)
  -> viewer-scoped ProjectionInput
  -> project(projectionInput, viewer, context)   [sandboxed, untrusted]
  -> ClientState
  -> defense-in-depth checks (structural, live; differential, offline at publish)
```

- **Visibility schema**: a new artifact-model field, `ProjectionVisibility` (addendum to `WORK-0044`, exact naming is Implementation Freedom), declared per state path, with three classes: `public`, `private:<owner-selector>` (for example, keyed by the participant identity that owns that subtree), and `server_only`. Exact schema serialization (JSON Pointer-style paths vs. a shape mirrored against the state's own structure) is Implementation Freedom, constrained only by needing to address arbitrary nesting depth in `state`.
- **Declassification primitives (starter set, Playhoot-computed, never author code)**: `count`/`length` (size of an array/string at a private path), `exists`/`nonEmpty` (boolean presence check), `redacted` (a fixed placeholder value, e.g. `"HIDDEN"`, standing in for a private value's presence without revealing content or size). An author declares which primitive applies to which private path and under what exposed name; Playhoot computes it during filtering and places the result in `ProjectionInput` at that name, alongside the (still-excluded) raw private data.
- **Filtering step**: a new Session Runtime component (package placement is Implementation Freedom) that walks `state` once per viewer using the declared schema, producing `ProjectionInput` containing: everything declared `public`; everything declared `private` and owned by the requesting viewer; every declared declassified derivative computed from any path (owned or not); and nothing else. `server_only` and any other viewer's undeclared/plain `private` data are always absent, never merely redacted-in-place.
- **Structural check**: after `project` returns `ClientState`, recursively scan it for any value equal to a value found at a `private`-to-someone-else or `server_only` path in the authoritative `state` (not `ProjectionInput`, which by construction never contains it) — this is intentionally checking against the *authoritative* state, since a filtering bug is exactly what this check must catch.
- **Differential check (offline, publish-time)**: for representative fixture states, for each viewer V and each path declared private to some other owner, perturb that path and re-run `project(filter(state, V), V, ctx)`; if V's `ClientState` changes, confirm the changed portion corresponds to a declared declassified derivative of that path — if not, flag it. This validates the schema's own declarations are consistent with what the script actually does, not merely that they were stated.

## Constraints and Invariants

- `project` must never receive, in its `ProjectionInput`, any state data the viewer is not authorized to see except explicitly declared `public` data, the viewer's own `private` data, and explicitly declared declassified derivatives. This is enforced by construction (data absence), not by post-hoc checking.
- `server_only` data must never appear in any `ProjectionInput`, for any viewer, under any circumstance.
- A differential/causal check must never treat an output difference as a leak when that difference is fully explained by an already-declared public/declassified dependency.
- Retrieving current view must not depend on replaying prior messages/effects — it derives from `WORK-0038`'s persisted state plus the requesting viewer's identity, through the filtering step.

## Acceptance Criteria

- For any state with a path declared `private` to viewer W, invoking the filter-then-project path for any other viewer V never places that path's raw value anywhere in V's `ProjectionInput`.
- A declared declassified derivative (for example, a `count`) is present in every non-owning viewer's `ProjectionInput` at its declared name, computed by Playhoot, without the underlying private value itself being present.
- `server_only` data is absent from every viewer's `ProjectionInput`, including the owner's own.
- The structural check flags a computed `ClientState` that contains a value equal to another viewer's private or server-only authoritative-state data.
- The offline differential check flags an output difference between viewers unless it is fully explained by a declared public/declassified dependency; it must not falsely flag a legitimate declassified derivative as a leak.
- Retrieving a viewer's current `ClientState` on load/reconnect succeeds directly from `WORK-0038`'s persisted state with no replay of prior turns/events.
- Adversarial test: a script that attempts to reference another viewer's excluded data receives no such data at all (verified by inspecting `ProjectionInput`, not merely `ClientState`); a script exercised against a deliberately over-broad schema is still caught by the structural or differential check.

## Implementation Freedom

- Exact `ProjectionVisibility` schema field name/serialization shape, package placement of the filtering step, and whether the structural/differential checks live in one shared package or two.
- Exact fixture-state provisioning mechanism for the offline differential check (author-supplied vs. Playhoot-generated) — bounded by needing at least one representative state per distinct visibility declaration.

## Verification

- Unit tests for the filtering step covering `public`/`private`/`server_only`/declassified-derivative classes independently and in combination.
- The adversarial-authored-script test from Acceptance Criteria, run against a real sandboxed `project` call, not a mock.
- A test proving the differential check does not false-positive on a correctly-declared declassified derivative (directly addressing the flaw an output-variance-only check would have).
- `go test ./session/...` / `go build ./...` / `go vet ./...` clean, per this Project's existing convention.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new package-local contract document recording the visibility/declassification model, the filtering step, both defense-in-depth checks, and their respective limits.
- A dated addendum to `session/docs/GAME_VERSION_ARTIFACT_MODEL.md` (`WORK-0044`'s own record) adding `ProjectionVisibility`, mirroring the precedent already set by `ParticipantConstraints` — not a rewrite of that already-DONE WORK.
- `session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md` and `session/internal/executor/LOGICAL_CONTRACT.md` gain the second entry point and its `ProjectionInput` shape.

## Blockers

None. `WORK-0038`/`WORK-0039` are DONE; both Material Decisions below are resolved.

## Material Decisions (Resolved 2026-09-29)

1. **Does this WORK also wire `project` end to end (sandbox contract, Executor gRPC surface, Session-side port, call site), or does that split into its own WORK first?** Resolved: yes, keep it in this WORK — one additional RPC/method on an already-deployed, already-reviewed service, reusing the already-built/hardened worker pool and resource limits; no other WORK claims it.
2. **What does the privacy mechanism actually check, and against what ground truth?** Resolved, by explicit human decision, as a fourth option beyond the three originally presented (structural-only, differential-only, or a hybrid of the two as the *primary* mechanism): privacy is primarily **capability-based**, not detection-based. `project` receives a Playhoot-constructed, viewer-scoped `ProjectionInput` — built by a visibility-filtering step against a newly declared three-class schema (`public`/`private`/`server_only`) plus an explicit declassification mechanism for authorized derivatives — and never receives excluded data at all, rather than receiving everything and being checked afterward. The originally proposed structural and differential checks are kept, but explicitly demoted to defense-in-depth against a filtering-step bug or a schema-authoring mistake, never relied on to compensate for over-broad script access. The human decision also corrected a real flaw in the originally proposed differential check: an output difference caused by a *legitimate* declared derivative (for example, an opponent's card count legitimately depending on their hand) is not itself a leak, so any differential test must be evaluated against the declared visibility/declassification model, not a bare "did the output change" test.

## Scope Narrowing (2026-09-29, implementation-time, flagged not silently dropped)

The offline/publish-time differential check named in Scope item (b), Approved Design, and Acceptance Criteria/Verification above was **not implemented in this pass**. Everything else in Scope was: the visibility schema, the declassification mechanism, the Playhoot-side filtering step (the primary, capability-based guarantee), `project` wired end to end, the live structural containment check (defense-in-depth), and `GetClientState` retrieving a viewer's current `ClientState` directly from persisted state with no replay.

This is a real reduction from the originally drafted scope, not an oversight - flagged explicitly rather than silently closing this WORK as if the differential check existed. Rationale: the primary guarantee (capability-based filtering) and one defense-in-depth layer (the live structural check) are both complete, tested, and sufficient on their own per this WORK's own Approved Design language ("NOW-justified on its own"); the differential check was explicitly the *stronger but costlier* addition ("SOON/LATER" in the same anti-overengineering framing), and building it well (real fixture-state provisioning, a real offline harness invoking the actual sandboxed Executor) is a meaningfully sized addition of its own. Deferring it does not weaken the privacy boundary this WORK actually delivers: `project` never receives excluded data regardless of whether the differential check exists, since that guarantee comes from the filtering step, not from either defense-in-depth check.

Recorded in `session/workflows/sessionlifecycle/internal/visibility/LOGICAL_CONTRACT.md`'s own "Explicitly Not Implemented Here" section as a tracked gap. Building it is a small, well-scoped candidate follow-up (a new package invoking `Filter`+the real `executor.Executor.Project` against author/Playhoot-supplied fixture states at Game-version-publish time) - not raised here as a new WORK number, since this Project's own convention reserves that for the human/Conversational-AI-led triage this Completion Record's own review should prompt.

## Completion Record

Implemented per Approved Design, with the one flagged narrowing above. All of `session/jsexecutor` (sandbox `project` script contract sharing `execute`'s worker/isolation model, `executor.proto`'s new `Project` RPC, `grpcserver.Server.Project`), `session/internal/executor` (`ProjectionInput`/`ProjectionOutput`, `Executor.Project`, `GRPCClient.Project` with the same `Unavailable`-only retry policy as `Execute`, `Fake.ProjectFunc`), and a new `session/workflows/sessionlifecycle/internal/visibility` package (`Schema`/`Rule`/`Class`/`DeriveFunc`, `Filter`, `LeakCheck`, `ParseSchema`) were added. `session_game_version_artifacts.projection_visibility` (`JSONB NULL`) persists the author's declared schema, migration `20260929000005_session_game_version_artifacts_projection_visibility`. `sessionlifecycle.Manager.GetClientState` is the new public entry point: an unlocked, non-transactional read (`ResolveSessionForClientState`, and `FindActor`/`GetRuntimeTurn`/`ResolveGameVersionArtifact` reused against the plain DB handle) that resolves the viewer's `ActorRef`, runs `visibility.Filter` against the pinned artifact's schema, calls `executor.Project`, and runs `visibility.LeakCheck` afterward purely as a defense-in-depth alert, never gating the returned result on it.

A real bug was caught and fixed during implementation, not left for review: the declassification synthesis pass initially enumerated instantiations against a declared derivative's own destination `Path` (a synthesized field name, e.g. `players.*.handCount`) instead of its `Source` (the real private field, e.g. `players.*.hand`) - since the destination name does not exist anywhere in real state, this produced zero declassified values at all. Caught by this WORK's own test suite, not discovered later; fixed by enumerating against `Source` and substituting its bound wildcard segment into `Path` instead.

Two engineering-standard violations were caught and fixed before considering this WORK complete: several new comments cited "WORK-0041" directly as justification (this repository's own comment standard, mechanically enforced by `TestNoInternalDocCitationsInComments`, requires the reasoning stated directly instead - all such citations across `session/internal/executor`, the new migration, `session/jsexecutor/internal/sandbox/worker.go`, `session/types.go`, `client_state.go`, `internal/repo`, and `internal/visibility` were reworded), and `GetClientState`'s own exported doc comment referenced the internal `visibility` package by name (this repository's separate "Public API Comments Stay At The Public Contract" standard, mechanically enforced by `TestExportedDocCommentsStayAtPublicContract`, forbids this - reworded to describe the behavior without naming the package).

Verification: `go build ./...`/`go vet ./...` clean repository-wide. `go test ./...` clean except the same two confirmed pre-existing, unrelated failures this Project's history already documents repeatedly (`comment_standard_test.go`'s three citations in an untouched migration file; `TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration`, the already-documented `session-runtime-v1`-owned Join/Leave race) - both re-confirmed pre-existing here via `git stash` against this WORK's own pre-change baseline, not merely assumed from history. A reachable Postgres container was available this session; the new migration and every touched repo method were exercised against it directly (`session/workflows/sessionlifecycle/internal/repo`, `internal/visibility`, and the full `sessionlifecycle` package all pass against real Postgres). New test coverage: `internal/visibility/visibility_test.go` (9 unit tests covering `Filter`'s public/private/server_only/declassified classes independently and in combination, an undeclared-path-defaults-to-excluded case, `LeakCheck`'s verbatim-leak detection, its non-false-positive behavior against a legitimate declassified derivative, and its short-value noise filter, plus `ParseSchema`'s empty-is-maximally-restrictive default); `session/workflows/sessionlifecycle/client_state_test.go` (6 mock-based scenarios covering every `GetClientStateOutcome`, including one asserting the Executor's own fake never receives a non-owning viewer's private data at all, not merely that it goes unused).

Documentation synchronized: `session/workflows/sessionlifecycle/internal/visibility/LOGICAL_CONTRACT.md` (new), `session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md` and `session/internal/executor/LOGICAL_CONTRACT.md` (both gain the `project`/`Project` entry point), `session/docs/GAME_VERSION_ARTIFACT_MODEL.md` (dated "Projection Visibility" addendum, mirroring the `ParticipantConstraints` precedent, plus the artifact-shape summary table/prose updated to list the new optional field). Next: independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, which should also triage the deferred offline differential-check follow-up named in Scope Narrowing above.
