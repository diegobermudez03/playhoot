# SESSION-ADR-0025: Snapshot-Based Session Runtime Persistence (Reverting Replay-First)

Status: ACCEPTED
Created: 2026-09-27
Last status change: 2026-09-27
Supersedes: SESSION-ADR-0023 (the central decision only — that Session Runtime never persists a full state Snapshot and instead relies on deterministic replay of the durable signal log as the live-correctness mechanism. The conceptual existence of `session_runtime_turns` as the ordered durable-cause envelope, the archive-destination decision (same-database PostgreSQL JSONB, not object storage), and the "derived state is not independently durable truth" framing for Presentations/Effects are unaffected and are restated here)
Superseded by: None
Legacy ID: SESSION-ADR-0025

## Context

`SESSION-ADR-0023` accepted persisting no full Snapshot per Turn, relying instead on Game Language's engine being provably, closedly deterministic (no wall clock/network/OS randomness reachable from authored code, enforced by the DSL's closed operation set) to safely reconstruct current/historical state by replaying the durable signal log on demand. `docs/projects/completed/game-language-flat-execution-model/works/WORK-0028-engine-owned-turn-execution.md` (via `GAME-ADR-0027`) implemented this fully: `session_runtime_turns` carries no Snapshot column, and `reconstructCurrentSnapshot` replays durable state on every read, verified against real Postgres.

`docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md` and `GAME-ADR-0028` now replace Game Language's closed DSL with sandboxed JavaScript. JavaScript's determinism can be encouraged (constraining authored code's capabilities, static-analysis/lint enforcement flagging use of wall-clock/random/network APIs, sandboxing removing host access) but not guaranteed the way the DSL's closed type/operation set guaranteed it by construction — a lint pass cannot exhaustively prove a general-purpose language's execution is pure. Relying on replay as the *live-correctness* mechanism against an engine that is only best-effort deterministic would mean an undetected nondeterminism bug could silently produce a different reconstructed state than what actually executed live — an unacceptable correctness risk for the mechanism `SESSION-ADR-0023` built.

## Decision

### The current confirmed state is the authoritative record

Session Runtime persists the full current state (the JavaScript execution boundary's `newState`, per `GAME-ADR-0028`) as part of every committed Turn, and treats that persisted state — not a replay reconstruction — as the authoritative source for continuing a Session (a resumed process, a reconnecting/disconnected player, or any other operation that needs "what is the Session's state right now"). Determinism of the authored script is a best-effort authoring-quality property (encouraged by sandboxing and static-analysis lint enforcement, per `ADR-0015`/`GAME-ADR-0028`), not the mechanism live correctness depends on.

### The durable signal/event log is retained, for reconstruction only

Session Runtime continues to durably persist the ordered sequence of external/runtime causes that produced each Turn (interaction responses, timer expirations, user intents, session lifecycle signals) — the same category of information `SESSION-ADR-0023`/`SESSION-ADR-0006` already established durable representations for (`session_runtime_turns`, `session_interactions`, `session_timer_obligations`, `session_cause_events`, `session_runtime_starts`). This is retained not as the live-correctness mechanism, but so that a best-effort session reconstruction/replay capability can eventually reconstruct how a Session unfolded, turn by turn, for audit/inspection/debugging purposes. A divergence between a reconstruction attempt and the originally persisted state is a diagnostic finding about that reconstruction attempt, not a live-Session correctness failure — the live path never depends on that reconstruction succeeding.

### What changes concretely versus `SESSION-ADR-0023`

- `session_runtime_turns` (or its successor table, per the owning WORK's naming) gains back a persisted state/snapshot representation — the artifact `SESSION-ADR-0023` specifically removed. Its exact column shape (full state per Turn, or a more compact representation) is the owning WORK's design decision.
- Runtime state reconstruction after process loss, or when a session resumes, loads the current persisted state directly; it does not replay the signal log to obtain it. This directly reverses `SESSION-ADR-0023`'s "Runtime state reconstruction after process loss" section.
- The signal/event log's purpose changes from "the thing current state is computed from" to "durable history kept for an eventual best-effort reconstruction feature" — a capability requirement (see `docs/projects/active/js-runtime-migration/works/`), not yet an implemented feature.
- The archive/history-compaction direction `SESSION-ADR-0023` accepted (same-database PostgreSQL JSONB, not object storage) is unaffected and restated: an archived Session's record still needs enough durable information (pinned artifact identity, the full signal/event log, terminal metadata) to support the same best-effort reconstruction after archival, plus now also the final persisted state itself.

## Rationale

Once the executing language's determinism can no longer be treated as a closed-form guarantee, replay stops being a safe *correctness* mechanism and becomes, at best, a best-effort *diagnostic* one — continuing to rely on it for live correctness would mean a single nondeterminism bug in authored code silently corrupts what every future operation believes the Session's state is, discovered only if/when a reconstruction happens to be attempted and happens to diverge. Persisting the actual state the engine produced removes that entire class of risk: current state is always exactly what live execution actually computed, never a hopeful re-derivation of it.

Retaining the full signal/event log despite no longer needing it for correctness preserves optionality the product genuinely wants (an eventual session reconstruction/rewatch capability, explicitly named as a goal) without making that optional capability a load-bearing part of every Session's live correctness.

## Alternatives Considered

### Keep replay-first, pin an exact sandboxed JavaScript runtime build per authored version

Rejected for V1, per `ADR-0015`'s equivalent alternative — this trades an open-ended operational pinning/support obligation for a correctness guarantee this record's snapshot approach no longer needs. Not forbidden to reconsider later if a concrete need arises; not required now.

### Persist a snapshot only periodically (for example, every N turns) and replay the remainder

Rejected. This reintroduces a partial dependency on replay for live correctness (reconstructing the turns since the last snapshot), which reintroduces exactly the determinism-dependency risk this record is meant to remove, for a storage saving that is not currently a demonstrated problem at this product's scale (per the actual-usage evidence found during this migration's own research: `session_runtime_turns` currently reaches `sequence=2` for a real session).

### Stop persisting the signal/event log entirely, keep only current state

Rejected. This forecloses the product's own stated goal of eventually reconstructing/rewatching a session's history, which the mandate driving this migration explicitly requires remaining possible, even on a best-effort basis.

## Consequences

- A new WORK (`docs/projects/active/js-runtime-migration/works/WORK-0038-snapshot-based-session-runtime-persistence-migration.md`) owns designing and implementing this reversal: reintroducing a persisted-state column/table, removing live-path dependence on `reconstructCurrentSnapshot`-style replay, and defining the retained log's exact reconstruction-capability shape.
- `docs/projects/active/session-runtime-v1/works/WORK-0017-*` (Archival, PLANNED) is affected — its design must be revisited against this record rather than `SESSION-ADR-0023`'s archive-payload shape, since the archived payload now also needs the final persisted state, not only the replay-input log. Flagged in `session-runtime-v1`'s own `PROJECT.md`, not resolved here.
- This is, explicitly, an architectural reversal of a fully implemented, independently reviewed, DONE migration (`docs/projects/completed/game-language-flat-execution-model/works/WORK-0028-engine-owned-turn-execution.md` and the underlying `WORK-0019`). Per `docs/decisions/README.md`'s historical-immutability rule, those WORK specifications and `SESSION-ADR-0023`/`GAME-ADR-0027`'s own historical text are not rewritten to hide that a reversal occurred; this record is the new decision superseding the relevant portion going forward.
- `SESSION-ADR-0023`'s still-open "replay compatibility across future compiler/engine builds" question (its own final "Not Yet Decided" item) is moot for live correctness under this record, since replay is no longer the correctness mechanism; it remains a relevant question only for the best-effort reconstruction capability, at a much lower severity.

## Canonical Knowledge Impact

- `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` — replaces the replay-first sections with this model, once the owning WORK lands.
- `game/README.md` — Session Runtime Turn And Persistence Model, Process-Agnostic Recovery, and History Archival Direction sections updated to reference this record alongside `SESSION-ADR-0006`/`SESSION-ADR-0023`.
- `session/docs/decisions/SESSION-ADR-0023-replay-first-session-runtime-persistence.md` — `Superseded by:` header annotated.
- `game/docs/decisions/INDEX.md` — this record added, with a supersession annotation for `SESSION-ADR-0023` matching the existing precedent for partial supersessions in this index.
- `docs/projects/active/session-runtime-v1/PROJECT.md` — flagged (not resolved) as affecting WORK-0017.

## Implementation Impact

Not authorized by this record. Routed to `docs/projects/active/js-runtime-migration/works/WORK-0038-snapshot-based-session-runtime-persistence-migration.md`.
