# WORK-0037: Deterministic-Authoring Static Analysis / Lint Enforcement

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-29 (IMPLEMENTING -> DONE: independent review APPROVED after two fix/re-review rounds)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`
- `session/docs/decisions/SESSION-ADR-0025-snapshot-based-session-runtime-persistence.md`

Canonical context:
- `session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md` (the actual current sandbox contract this WORK validates authored scripts against — read during drafting, not assumed).
- `session/jsexecutor/internal/sandbox/determinism.go` (the determinism prelude; shows `Date`/`Math.random`/`performance`/`os` are already neutralized at runtime).

## Outcome

Because sandboxed JavaScript's determinism cannot be guaranteed the way Game Language's closed DSL guaranteed it by construction, this WORK builds a required-before-acceptance validation pass over authored/generated **backend script** source (frontend script is explicitly out of scope — see Scope) before that script is accepted for publish or use. This is a best-effort authoring-quality gate, explicitly not a substitute for the isolation boundary `WORK-0036` enforces at runtime — it exists because `SESSION-ADR-0025`'s snapshot-based persistence model no longer depends on perfect determinism for correctness, but reducing nondeterminism and confusing authoring surprises still matters for the best-effort session-reconstruction capability (`WORK-0051`) and for authoring quality generally.

## Context

This WORK's original Outcome assumed a lint was needed to catch calls to "known nondeterministic host APIs (wall-clock reads, `Math.random`, network, filesystem...)". Drafting checked that assumption against the actual implemented sandbox (`WORK-0035`/`WORK-0036`, both DONE) rather than trusting the assumption:

- `session/jsexecutor/internal/sandbox/determinism.go`'s prelude already replaces `Date`/`Math.random` with context-seeded deterministic substitutes, and removes `performance`/`os` from global scope entirely, before any authored code runs. Calling these is already safe/deterministic, not a nondeterminism bug.
- No filesystem/network/process/environment host capability is wired into the sandbox's JS globals at all (`LOGICAL_CONTRACT.md`'s Isolation Guarantees). Calling `fetch`/`require`/`fs.*`/etc. already throws a `ReferenceError` inside the script, which the existing sandbox already surfaces as a clean `*ScriptRejectedError` — not a silent nondeterminism bug, just a runtime failure discovered late.

So the literal APIs this WORK's original Outcome named are already handled at the runtime layer. This WORK's real remaining value, confirmed by explicit human decision (see Material Decisions below), is authoring-quality shift-left feedback, not a runtime-correctness gap.

## Material Decisions — Resolved (2026-09-29)

1. **Lint scope**: both of the following, as full v1 scope:
   - Flag references to any global/API not in the sandbox's actual supported surface (for example `fetch`, `require`, `process`, `fs`, `WeakRef`, `Intl` if actually absent from the pinned QuickJS-on-`wazero` build — see Implementation Freedom) as an **ERROR**-severity finding: this is guaranteed to fail at runtime today, and catching it at authoring/publish time is strictly better than a live `ReferenceError` discovered later.
   - Flag direct use of `Math.random()`/`Date()`/`Date.now()`/`new Date(...)` as a **WARNING**-severity finding: safe and deterministic today, but an author relying on real randomness or a real wall-clock deadline will get silently confusing behavior (logical-time/seed-derived values, not real time/randomness), so it is worth surfacing even though it does not fail.
2. **Implementation approach**: a real JS parser/AST library, not a token/regex scan. No JS parser dependency exists in this repository today (only `github.com/fastschema/qjs`, the QuickJS-on-`wazero` execution binding, which is not itself a usable static-parse API for this purpose). This is a new external Go dependency — exact library choice is Implementation Freedom (see below), but the class of dependency itself is approved.

## Scope

### In Scope

- A Go package/function validating one **backend script**'s source text and returning a list of findings (each with severity, a human-readable message, and a source position when the parser exposes one) plus a distinguishable parse-failure outcome for syntactically invalid source — never a panic.
- AST-based detection of:
  - Any identifier/member reference not in an explicit **allow-list** matching the sandbox's actual supported global surface (allow-list, not a deny-list of "known bad" names — a deny-list cannot anticipate every name a future binding upgrade might add or remove; see Constraints on keeping this in sync).
  - Direct references to `Math.random`, `Date`, `Date.now`, and `new Date(...)` specifically (WARNING severity, distinct from the ERROR severity used for the allow-list violation above).
- A severity model: `ERROR` findings mean this script is guaranteed to fail at runtime under the current sandbox contract; `WARNING` findings mean the script is safe but likely to surprise its author. ERROR findings must block acceptance under Constraints below; WARNING findings are returned to the caller but do not themselves block acceptance (an author may deliberately want a deterministic-but-repeatable pseudo-random sequence, which this substitution actually provides).
- Being an importable capability a caller in another domain (Orchestrator, per `WORK-0033`'s own future scope) can invoke without depending on the sandbox/`jsexecutor` gRPC service at all — this is pure static analysis over source text, not an execution call.

### Out of Scope

- **Frontend script** validation. Frontend script runs in a browser iframe (`ADR-0015`) with an entirely different, legitimate global surface (DOM, `fetch`, etc. may be genuinely appropriate there) — this WORK's allow-list is specific to the backend sandbox's own supported profile and must not be applied to frontend script.
- The publish/authoring HTTP path, orchestration, and where exactly in that flow this validation is invoked, and how a WARNING finding is surfaced to a human author (a UI/response-shape concern) — `WORK-0033`'s own scope. This WORK exposes the validation capability; it does not own a caller.
- Any change to the sandbox's own runtime isolation/enforcement — unchanged, owned by `WORK-0035`/`WORK-0036`.
- Exhaustively re-probing the QuickJS-on-`wazero` binding's entire global surface beyond what `LOGICAL_CONTRACT.md` already documents, except where this WORK's own allow-list construction requires confirming a specific name's presence/absence (see Implementation Freedom).
- Optional editor/IDE-integration linting — this is a required-before-acceptance gate per Constraints, not an optional authoring-time suggestion, though nothing prevents the same capability being reused as one later.

## Approved Design

- A new, non-`internal` Go package (exact location/name is Implementation Freedom, guided by `docs/engineering/standards/domain-logic-placement.md`) exposing a function shaped like `Validate(source string) (Result, error)`, where `Result` carries an ordered list of findings (`Severity`, `Message`, and a line/column position when the parser provides one) and `error` is reserved for a script that fails to parse at all (a distinguishable outcome, not a Go panic — mirroring the `*ScriptRejectedError`/`*WorkerExecutionError` distinction pattern `WORK-0035` already established).
- The allow-list is derived from, and must stay consistent with, `session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md`'s own Isolation Guarantees — this package becomes the second place (besides the sandbox itself) that encodes "what backend scripts may actually reference," so it must reference/cross-check that document rather than silently drift from it (One Fact, One Owner).
- ERROR-severity findings (allow-list violations, parse failures) constitute the "reject before acceptance" outcome the Outcome section requires. WARNING-severity findings (`Math.random`/`Date` usage) are informational only.

## Constraints and Invariants

- Must not be presented as a correctness guarantee — its own Documentation Impact must state plainly that it reduces, not eliminates, nondeterminism/authoring-surprise risk (per `ADR-0015`'s Rationale, which already states a lint pass cannot fully police a general-purpose language).
- Must run as part of script validation/publish (coordinating with `WORK-0033`/`WORK-0044`), not only as an optional authoring-time suggestion — concretely, an ERROR-severity finding must be capable of blocking acceptance once a real caller (`WORK-0033`) exists; this WORK's own package must make that distinction available, even though it does not itself own the acceptance gate.
- Must apply only to backend script content, never frontend script.
- The allow-list must be revisited if `session/jsexecutor`'s own sandbox surface changes (a new global removed/added, a binding upgrade) — this is a coordination dependency this WORK cannot unilaterally guarantee holds forever; flagged, not solved, here.

## Acceptance Criteria

- A backend script referencing `fetch`, `require`, `process`, `fs` (or any property access on an undefined `fs`), `XMLHttpRequest`, or `WebSocket` produces an ERROR-severity finding identifying the disallowed reference and its source position.
- A backend script calling `Math.random()`, `new Date()`, `Date.now()`, or constructing `Date` with arguments produces a WARNING-severity finding explaining these are deterministically substituted at runtime, not real randomness/time.
- A backend script using only the standard ECMAScript surface plus the documented `execute(previousState, event, context)` contract (`session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md`'s Script Contract) produces zero findings.
- A syntactically invalid backend script produces a distinguishable parse-failure outcome, never a Go panic.
- `Validate` never executes the script it analyzes (pure static analysis — no sandbox/worker process is spawned).

## Implementation Freedom

- Exact JS parser/AST library (a widely used, actively maintained pure-Go option — for example a parser-only import of `github.com/dop251/goja`'s `parser`/`ast` packages, or an equivalent) is the implementer's choice.
- Exact package name/location, `Result`/`Finding` type shapes, and exact rule/message wording are the implementer's choice.
- Constructing the exact allow-list (which ECMAScript intrinsics are genuinely reachable/unmodified versus removed/replaced by the sandbox) should be empirically confirmed against the actual pinned `qjs` binding where `LOGICAL_CONTRACT.md` does not already state it definitively, following this project's own established practice of verifying binding behavior rather than assuming it (see that document's own "every guarantee below was independently verified" framing) — not required to be exhaustive beyond what a reasonable authoring surface would plausibly reference.

## Verification

- `go test ./...` for the new package, covering every Acceptance Criterion above with fixture scripts (at minimum: one ERROR-triggering script per disallowed API named above, one WARNING-triggering script per `Math.random`/`Date` form, one clean script, one syntactically invalid script).
- `go build ./...` / `go vet ./...` clean repository-wide.
- No independent review protocol decision has been made yet for this WORK specifically; follow `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`'s normal criteria (a new external dependency and a new authoring-facing contract both argue for it, consistent with `WORK-0035`/`WORK-0036`'s own precedent).

## Documentation Impact

### Accepted / Canonical Knowledge

- A new authoring-facing document (successor role to the retired `game/language/v1/program/DEFINITION.md`'s "what is/isn't buildable" framing) under `session/docs/`, stating which APIs are disallowed/discouraged for backend scripts and why, cross-referencing `session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md` as the technical source of truth rather than duplicating it.

### Current-State Documentation After Implementation

- `session/CURRENT_STATE.md` — new validation capability's status added.

## Blockers

- None. (Prior blocker — `WORK-0035`'s supported-JavaScript-profile decision — is resolved; `WORK-0035` and `WORK-0036` are both DONE.)

## Completion Record

Implemented as designed. New package `session/usecases/scriptlint` (`scriptlint.go`, `allowlist.go`, `walk.go`): `Validate(source string) (Result, error)` parses a backend script with `github.com/dop251/goja`'s `parser`/`ast` packages only (the VM itself is never imported anywhere in the repository - confirmed by review) and reports `ERROR` findings (a reference outside the sandbox's actual supported global surface) and `WARNING` findings (safe but likely to surprise an author: `Math.random`, `Date`/`Date.now`/`new Date`, and - a local addition beyond the two Material Decisions below, discovered by the same empirical probe and folded into the already-approved WARNING category rather than treated as a new decision - `setTimeout`/`setInterval`/`queueMicrotask`, since the sandbox's synchronous `execute()` call never drains pending timers/microtasks before capturing its result). New authoring-facing doc: `session/docs/BACKEND_SCRIPT_AUTHORING_CONSTRAINTS.md`. `session/CURRENT_STATE.md` synchronized with a new capability row.

The allow-list was not assumed from `LOGICAL_CONTRACT.md` alone - it was empirically re-verified by running a probe script through the real sandbox (`Object.getOwnPropertyNames(globalThis)`, walking the prototype chain) before implementation, following this Project's own established practice of verifying binding behavior rather than trusting documentation. That probe found `std` is a live, fully reachable global beyond what `LOGICAL_CONTRACT.md` names (it only documents `std.getenv`'s neutralization, not that `std` itself remains present), plus several sandbox-implementation-specific globals with no legitimate authoring use (`bjson`, `print`, `scriptArgs`, `navigator`, `gc`, `QJS_PROXY_VALUE`, `DOMException`) - all excluded from the allow-list (`ERROR` if referenced). The probe also confirmed `Atomics` is genuinely absent from the sandbox's real global surface, resolving an independent reviewer's own NON_BLOCKING uncertainty about it (the reviewer could not re-run the probe itself under a read-only review constraint).

Independent review ran three rounds: round 1 CHANGES_REQUIRED (two REQUIRED_FIX - `globalThis.<name>`/`globalThis['<name>']` completely bypassed the ERROR-severity allow-list check, since member-access detection never treated `globalThis.X` as equivalent to a direct reference to `X`; a tagged template literal's tag expression was never walked by the AST walker at all), fixed same day with regression tests added. Round 2 CHANGES_REQUIRED (one new REQUIRED_FIX found on fresh inspection - `allowlist.go` never actually cross-referenced `session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md` by path, despite the Approved Design explicitly requiring it), fixed with a code comment. Round 3 **APPROVED, no findings** - the same reviewer re-verified the fix directly against the file content and by actually running `TestNoInternalDocCitationsInComments` itself (confirming the new citation doesn't trip that mechanical check, since it targets ADR/WORK/Blocker/Slice/`docs/work|engineering|ai/` patterns specifically, not an arbitrary package path, and the cited variables are unexported so the separate "no internal refs in exported doc comments" convention does not apply either).

Three NON_BLOCKING observations were logged across the review rounds, left open as informational, not required for this closure: (1) the flat single-scope shadowing model (the whole script is treated as one scope for bound-name collection, not JavaScript's real nested/block scoping) can theoretically under-flag a real violation if an unrelated name shadows a banned global elsewhere in the same script - an intentional, documented simplification, since it only risks under-flagging, never false-flagging a script's own legitimate local variable; (2) a `globalThis.X`/`globalThis['X']` finding's reported source position points at the `globalThis` token itself rather than the accessed property name - still a usable position, just less precise; (3) `scriptlint_test.go` does not have a fixture specifically for "`Date` constructed with arguments" as its own distinct case (the underlying detection already covers it correctly by construction, per direct code inspection - a test-completeness gap only, not a functional one).

Verification: `go build ./...`, `go vet ./...` clean repository-wide at every round. `go test ./...` clean except one confirmed pre-existing, unrelated failure (`TestNoInternalDocCitationsInComments` on `session/internal/storage/migrations/20260926000000_session_runtime_failures.go`, three ADR/WORK citations) - independently confirmed at every review round via `git status`/`git diff`/`git log` to be untouched by this WORK and to predate it (last touched by an unrelated prior commit). Every Acceptance Criterion is covered by a passing fixture test in `scriptlint_test.go`, including the two regression tests added for the round-1 fixes.

A real, pre-existing documentation/implementation drift was surfaced during review, confirmed genuine but out of this WORK's own scope: `session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md` states `performance`/`os` are "removed from the global scope entirely," but `determinism.go`'s actual prelude only assigns them `undefined` (an assignment, not a deletion) - both files have zero diff against this WORK's own starting point, so this is owned by already-DONE `WORK-0035`/`WORK-0036`, flagged here for whoever next touches that document/prelude rather than fixed as part of this WORK.

No caller invokes `Validate` yet - wiring it into an actual publish/authoring path is `WORK-0033`'s own future scope, as this WORK's Scope section already recorded before implementation began.
