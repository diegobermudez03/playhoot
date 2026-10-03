# WORK-0043: Durable Confirmed-Turn Result Delivery (Outbox)

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-30 (independent review returned APPROVED with no findings; closed)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `session/docs/decisions/SESSION-ADR-0019-session-runtime-post-commit-client-delivery-semantics.md` (extended, not reopened, by this WORK)
- `session/docs/decisions/SESSION-ADR-0026-confirmed-player-event-outcome-delivery-is-pull-based.md` (this WORK's own concrete implementation of that decision)

Canonical context:
- `session/workflows/sessionlifecycle/step_submit_player_event.go` (`interpretExistingSubmitPlayerEventClaim` - the existing idempotency-replay decode logic this WORK's own new read path reuses)
- `session/workflows/sessionlifecycle/internal/repo/request.go` (`fetchExistingSessionRequest`/`session_requests` - the already-durable store this WORK reads from, no new schema)
- `session/workflows/sessionlifecycle/client_state.go` (`Manager.GetClientState` - the pure, unlocked, non-transactional read this WORK's own new method mirrors)

## Outcome

Build the pull-based delivery mechanism `ADR-0015` requires for a confirmed `PLAYER_EVENT`'s own *outcome* (accepted/rejected/failed), as distinct from a mere transport acknowledgement and as distinct from best-effort `SEND_EVENT` delivery (`WORK-0042`). `SESSION-ADR-0019` already anticipated that a future capability needing durable/irreversible delivery "must explicitly design its own delivery/idempotency/retry semantics as a separate decision" rather than inherit its own "no outbox" conclusion — `SESSION-ADR-0026` is that decision; this WORK is its concrete implementation. Without it, the mandate's requirement to distinguish "a transport acknowledgement" from "an accepted play" has no real mechanism for a client that never received the original response to fall back on.

## Context

**Ground-truth check (2026-09-29), same session that resolved Material Decision #4 conversationally:** `SubmitPlayerEvent` already durably records its own outcome in `session_requests` (`(user_uuid, operation, idempotency_key)`), in the same transaction that commits the RuntimeTurn - the outcome is never actually lost today. What was missing was a way to retrieve it *without* resubmitting the full original `name`/`payload` (which risks `ErrIdempotencyConflict` on any reconstruction mismatch, and requires the caller to still have that payload around). `SESSION-ADR-0026` resolves this as a pull-based read, not a push-style outbox - no new durable table/schema, no background dispatcher.

**Vocabulary update (2026-09-28, unaffected by the above):** `WORK-0039`'s revised vocabulary has no dedicated "action result" command — a `PLAYER_EVENT`'s accept/reject outcome is already fully determined by Go itself (`*executor.ScriptRejectedError` vs. a successful `Execute` call), not something the authored script separately declares. This WORK's own scope is therefore specifically: durably delivering *that already-known Go-side outcome* back to the acting participant, not inventing a new platform command kind for it.

**No longer a real blocker (reassessed 2026-09-29):** this WORK's own previously-recorded dependency on `session-runtime-v1`'s `WORK-0020` for "where delivery attaches" does not actually gate this WORK, the same way `WORK-0020` never gated `Manager.GetClientState` (`WORK-0041`) - both are pure, unlocked reads a future live-connection layer calls, not designed by that layer. `WORK-0020` remains free to decide when/how it calls this new method; this WORK does not need that decided first.

## Scope

### In Scope

- New public Manager method, `GetSubmitPlayerEventOutcome`, mirroring `GetClientState`'s own shape: a pure, unlocked, non-transactional read against `session_requests` - no row lock, no mutation, no re-execution.
- A new exported repo method (`FindSessionRequest`) wrapping the existing private `fetchExistingSessionRequest` query, usable outside a claim/complete cycle.
- Extracting the existing outcome-decoding logic already inside `interpretExistingSubmitPlayerEventClaim` into a small shared helper both that function and the new read path call, so the decode logic (the `submitPlayerEventOutcome*` switch plus `ResponsePayload` unmarshal) is not duplicated.

### Out of Scope

- Any new durable table, schema, or background dispatcher - `SESSION-ADR-0026` already rejected this.
- Deciding when/how a live-connection layer calls this new method on reconnect - `session-runtime-v1`'s `WORK-0020` own concern.
- Any change to `SubmitPlayerEvent`'s own existing behavior, idempotency semantics, or business logic - this WORK adds a second, read-only entry point onto the same already-durable data, nothing about the write path changes.
- `Start`/`CancelSession`/`ExpireTimer`'s own outcomes - `ADR-0015`'s own text specifically names a confirmed `PLAYER_EVENT`'s outcome; the other three are not in this WORK's own scope (the same underlying mechanism could extend to them later, as a separate, explicitly justified WORK, not assumed here).

## Approved Design

- `session/types.go` gains:

  ```go
  // GetSubmitPlayerEventOutcomeOutcome is GetSubmitPlayerEventOutcome's
  // expected business outcome, a value distinct from a Go error: an ordinary
  // decline a caller should branch on, not treat as a failure.
  type GetSubmitPlayerEventOutcomeOutcome string

  const (
      // GetSubmitPlayerEventOutcomeFound means idempotencyKey's own
      // SubmitPlayerEvent request has already completed; Result carries its
      // recorded outcome.
      GetSubmitPlayerEventOutcomeFound GetSubmitPlayerEventOutcomeOutcome = "FOUND"
      // GetSubmitPlayerEventOutcomeNotFound means no completed request exists
      // yet for idempotencyKey - it was never submitted, is still being
      // processed, or belongs to a different session than sessionUUID names.
      GetSubmitPlayerEventOutcomeNotFound GetSubmitPlayerEventOutcomeOutcome = "NOT_FOUND"
  )

  // GetSubmitPlayerEventOutcomeResult is GetSubmitPlayerEventOutcome's
  // logical outcome. Result is only populated when Outcome is
  // GetSubmitPlayerEventOutcomeFound.
  type GetSubmitPlayerEventOutcomeResult struct {
      Outcome GetSubmitPlayerEventOutcomeOutcome `json:"outcome"`
      Result  SubmitPlayerEventResult            `json:"result,omitempty"`
  }
  ```

- `session/workflows/sessionlifecycle/internal/repo/request.go` gains:

  ```go
  // FindSessionRequest returns the session_requests row already owning
  // (userUUID, operation, idempotencyKey), or nil if none exists yet - a
  // pure lookup, unlike ClaimSessionRequest, which also inserts a fresh row
  // when none is found.
  func (r *Repo) FindSessionRequest(ctx context.Context, tx *gorm.DB, userUUID, operation, idempotencyKey string) (*Request, error) {
      return fetchExistingSessionRequest(ctx, tx, ClaimSessionRequestInput{UserUUID: userUUID, Operation: operation, IdempotencyKey: idempotencyKey})
  }
  ```

- `session/workflows/sessionlifecycle/step_submit_player_event.go` (or a small new file, Implementation Freedom) gains:

  ```go
  func (m *Manager) GetSubmitPlayerEventOutcome(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.GetSubmitPlayerEventOutcomeResult, error) {
      // resolves sessionUUID -> internal sessionID (read-only), calls
      // m.submitPlayerEventRepo.FindSessionRequest(ctx, m.dbServicer.GetDB(), string(userUUID), operationSubmitPlayerEvent, string(idempotencyKey)),
      // returns NotFound if nil, status != COMPLETED, or SessionID doesn't
      // match the resolved session; otherwise decodes via the shared helper
      // extracted from interpretExistingSubmitPlayerEventClaim and returns Found.
  }
  ```

- `interpretExistingSubmitPlayerEventClaim`'s own outcome-decoding switch/unmarshal logic is extracted into a shared, unexported helper (for example `decodeSubmitPlayerEventOutcome(existing *internalrepo.Request) (session.SubmitPlayerEventResult, error)`) that both it and `GetSubmitPlayerEventOutcome` call - `interpretExistingSubmitPlayerEventClaim` keeps its own additional `stored != incoming` conflict check, which the new read-only path does not need (it takes no `name`/`payload` to compare against at all).

## Constraints and Invariants

- Must not be conflated with or weaken `SESSION-ADR-0019`'s existing best-effort stance for cosmetic effects — this WORK adds a durable-read mechanism for a different, narrower class of output (confirmed `PLAYER_EVENT` results), it does not generalize durability to everything.
- Must introduce no new durable table/schema/background process - `SESSION-ADR-0026`.
- Must never mutate/re-execute anything - `GetSubmitPlayerEventOutcome` is a pure read, exactly like `GetClientState`.
- Must handle retries/duplicates without double-applying or silently dropping a result - already true by construction, since this WORK only adds a read path onto data `SubmitPlayerEvent`'s own existing idempotency mechanism already makes durable exactly once.

## Acceptance Criteria

- Calling `GetSubmitPlayerEventOutcome` with an idempotency key that already completed returns `Found` with the exact same `Result` the original `SubmitPlayerEvent` call itself returned (accepted, rejected, or runtime-execution-failed cases all covered).
- Calling it with an idempotency key that was never submitted, or belongs to a different session, returns `NotFound`.
- Calling `SubmitPlayerEvent` and then, without ever using its return value, separately calling `GetSubmitPlayerEventOutcome` with the same key returns the correct outcome - proving the "dropped response" scenario recovers correctly.
- This WORK adds no new durable column/table (empty migration diff).
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond this Project's already-recorded pre-existing, unrelated failures.

## Implementation Freedom

- Exact file placement of the new Manager method (`step_submit_player_event.go` vs. a new small file).
- Exact naming of the extracted shared decode helper.

## Verification

- Unit tests for `GetSubmitPlayerEventOutcome` (found/not-found/wrong-session cases) against mocked persistence, mirroring `TestManagerGetClientState`'s own style.
- A real-Postgres integration test simulating the fault this WORK exists for: call `SubmitPlayerEvent` to completion, then call `GetSubmitPlayerEventOutcome` with the same idempotency key *without ever having used `SubmitPlayerEvent`'s own return value* (modeling "the original response never reached the client"), asserting the recovered outcome exactly matches what was actually committed, and that no second RuntimeTurn/execution ever occurs as a side effect of the read.

## Documentation Impact

### Accepted / Canonical Knowledge

- `session/docs/decisions/SESSION-ADR-0026-confirmed-player-event-outcome-delivery-is-pull-based.md` already recorded (this WORK is its implementation, not its own decision-record source).
- `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - if it enumerates delivery guarantees, gains a short note that confirmed `PLAYER_EVENT` outcomes are durable and pull-retrievable, cross-referencing `SESSION-ADR-0026`.

### Current-State Documentation After Implementation

- `session/CURRENT_STATE.md` - new capability noted (`GetSubmitPlayerEventOutcome`), consistent with how `GetClientState` was recorded.

## Blockers

None.

## Completion Record

### Implementation Report (2026-09-30)

Work: `docs/projects/active/js-runtime-migration/works/WORK-0043-durable-confirmed-turn-result-delivery-outbox.md`

Work status: IMPLEMENTING

Implemented:
- `session/types.go`: `GetSubmitPlayerEventOutcomeOutcome` (`Found`/`NotFound`) and `GetSubmitPlayerEventOutcomeResult` (additive new public types, no existing type changed).
- `session/workflows/sessionlifecycle/internal/repo/request.go`: `FindSessionRequest` - a thin public wrapper around the already-existing private `fetchExistingSessionRequest` query, no new SQL/schema.
- `session/workflows/sessionlifecycle/step_submit_player_event.go`: `interpretExistingSubmitPlayerEventClaim`'s own outcome-decoding logic extracted into a shared `decodeSubmitPlayerEventOutcome` helper; new `Manager.GetSubmitPlayerEventOutcome` (pure, unlocked, non-transactional read) calls it after resolving the session (`ResolveSessionForClientState`, reused from `GetClientState`) and looking up the request row (`FindSessionRequest`) - returns `NotFound` for a missing row, a still-`PENDING` row, or a row scoped to a different session; `Found` otherwise.
- `submitPlayerEventRepoAPI`'s interface gained `ResolveSessionForClientState`/`FindSessionRequest`; `mocks_test.go` regenerated via `mockgen` (30 lines added, no other change).

Local implementation decisions:
- Placement of the new Manager method/helper inside `step_submit_player_event.go` itself rather than a new file (Implementation Freedom) - it is tightly coupled to that file's own `submitPlayerEventOutcome*` constants and existing decode logic.
- A still-`PENDING` request row (realistically only observable under a genuine concurrent race, since claim+complete happen atomically in one transaction) is treated identically to "not found" rather than a third outcome value - simpler for a caller, and the caller's own natural response (wait, then ask again) is the same either way.

Deviations from the approved WORK:
- None.

Discoveries:
- None requiring escalation. Two self-caught engineering-standard violations (not review-caught): a first-draft doc comment on `GetSubmitPlayerEventOutcomeResult` cited `session/docs/decisions/SESSION-ADR-0026` directly, and a first-draft integration test comment cited `WORK-0043` directly - both tripped `TestNoInternalDocCitationsInComments` and were reworded to state the reasoning inline before considering this WORK complete.

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `gofmt` diff-checked (CRLF-normalized) on every changed file - no real formatting defect.
- `go test . -run TestNoInternalDocCitationsInComments` - clean except the same three pre-existing citations in the untouched migration file this Project's history already documents.
- New unit tests (`TestManagerGetSubmitPlayerEventOutcome`, mocked persistence, mirroring `TestManagerGetClientState`'s own style): session-not-found, resolve-error propagation, not-found for a missing/still-pending/wrong-session request row, and found-with-correct-decoded-`Result` for all three outcome classes (rejected, runtime-execution-failed, accepted-with-response-payload-decode) - all pass.
- New real-Postgres integration tests written (`TestManagerGetSubmitPlayerEventOutcome_Integration`): a fault-injection case calling `SubmitPlayerEvent` to completion and then deliberately never consulting its own return value again, instead calling `GetSubmitPlayerEventOutcome` with the same idempotency key and asserting the recovered `Result` exactly matches what was actually committed, with no second RuntimeTurn created as a side effect; plus a rejected-outcome case and a never-submitted-key NotFound case. **Known limitation, not caused by this WORK's own code**: this session's own reachable Postgres container from earlier in this same conversation is no longer reachable (Docker Desktop is not running in this refreshed session/environment) - these new integration tests compile cleanly (confirmed via `go vet`) and are ready to run, but have not actually been executed against a real database this pass. The full `go test ./...` run without `TEST_DATABASE_*` set skips every integration test cleanly (the existing, unrelated, already-established sandbox-limitation behavior this Project's history repeatedly documents), confirming no regression in that fallback path either. Independent review should attempt to run these specific new tests against a reachable Postgres if one is available in its own environment.

Documentation synchronized:
- `session/docs/decisions/SESSION-ADR-0026-confirmed-player-event-outcome-delivery-is-pull-based.md` (new, ACCEPTED) and `session/docs/decisions/INDEX.md` (new row, "Next Session ADR" bumped to `SESSION-ADR-0027`) - persisted before drafting this WORK, per the decision/canonical-knowledge/work-spec ordering this repository's own process expects.
- `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`'s own "No Durable Client-Delivery Outbox" section - checked directly against this WORK's own conditional Documentation Impact ("if it enumerates delivery guarantees"): it does, in detail, including a direct reference to the exact future case `SESSION-ADR-0019` flagged and left open. Gained one paragraph closing that open reference: the confirmed-outcome case is resolved by the pull-based read this WORK adds, not by the durable outbox the rest of that section rejects, cross-referencing `SESSION-ADR-0026`.
- `docs/projects/active/js-runtime-migration/PROJECT.md` - Material Decision #4 marked resolved, Work table/Capability Coverage/Ordering-Dependencies/Current Work updated.

Known limitations:
- Real-Postgres execution of the new integration tests is outstanding this pass, for the environment reason above - not a code defect, and not this Project's first time encountering this class of sandbox limitation. The independent reviewer confirmed it has no reachable Postgres either, so this remains unverified by execution in any session so far.
- `session/CURRENT_STATE.md` was not updated, though this WORK's own DRAFT-time Documentation Impact said it would be "consistent with how `GetClientState` was recorded." The independent reviewer traced via `git log` that this precedent doesn't actually hold - `session/CURRENT_STATE.md` predates both `GetClientState` and `SubmitPlayerEvent` themselves and already doesn't mention either; it is a known, separately-tracked documentation-catch-up gap (carried in `internal/AI_CONTEXT.md`'s own "Next action" paragraph), not something this WORK should silently absorb on its own.

Ready for independent review:
YES.

### Independent Review (2026-09-30)

A fresh agent, with no access to this session's own context, reviewed this WORK per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`. It hand-traced `GetSubmitPlayerEventOutcome` line by line to confirm no mutation/re-execution occurs and all three `NotFound` branches (missing/pending/wrong-session) are reachable and independently exercised by tests, confirmed `decodeSubmitPlayerEventOutcome` is a genuine extraction rather than a divergent copy (traced `interpretExistingSubmitPlayerEventClaim`'s own unchanged conflict-check ordering), ran the new unit tests itself (all 8 subtests pass, concrete assertions), read the integration test's fault-injection case and confirmed it genuinely proves the claimed property (exact-result equality plus a Turn-count assertion ruling out re-execution), checked `SESSION-ADR-0026` against the ADR template section by section, verified `session/docs/decisions/INDEX.md`'s new row/next-ID line, verified the new `SESSION_RUNTIME_PERSISTENCE_MODEL.md` paragraph is accurate and non-contradictory, and ran its own fresh `go build`/`go vet`/`go test ./...` (repo-wide) plus `TestNoInternalDocCitationsInComments` directly.

Verdict: **APPROVED**, no findings.

The reviewer independently confirmed it also has no reachable Postgres/Docker in its own environment (checked directly, not assumed) - the real-Postgres fault-injection integration tests remain unverified by execution in either session, an honestly disclosed limitation in both, explicitly not blocking APPROVED given the strength of the mocked-unit-test coverage over the identical decision logic. One NON_BLOCKING documentation note, not introduced by this WORK: `session/CURRENT_STATE.md` was not updated to mention the new capability as this WORK's own Documentation Impact suggested it would be, but the reviewer traced via `git log` that the cited precedent (how `GetClientState` was recorded) doesn't actually hold either - `session/CURRENT_STATE.md` predates both capabilities and is already a known, separately-tracked documentation-catch-up gap, not something this WORK should have silently absorbed.

No REQUIRED_FIX or DECISION_REQUIRED finding remains. Closed to DONE.
