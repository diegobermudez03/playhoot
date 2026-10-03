Process: Engineering Standard
Topic: Behavior locality; Workflow vs. Use Case classification; repository sharing/extraction boundary
Current stage: DONE — standard persisted, human decision already given, no open checkpoint
Current execution surface: CODEBASE AGENT
Parent process: none (standalone Engineering Standard change); triggered by ambiguity surfaced during `session-runtime-v1` Slice 1 implementation
Return to: none — this process is terminal. The triggering initiative (`session-runtime-v1`) continues independently; see its own workspace for the routed migration.
Related durable artifacts: `docs/engineering/standards/domain-logic-placement.md`, `docs/engineering/standards/repositories.md`, `docs/engineering/standards/INDEX.md`, `docs/ai/workspaces/active/session-runtime-v1/AI_CONTEXT.md` (mandatory targeted migration recorded there)
Blocked by: nothing
Next action: none for this process. The Session Runtime migration this standard requires is routed separately through Feature Development/implementation against `session-runtime-v1` — do not perform it from this workspace.
Last durable checkpoint: standard wording persisted 2026-09-09 (see below).
Last updated: 2026-09-09

# Resume Context

## Problem

`session-runtime-v1` Slice 1 (WORK-0001) implemented `CreateSession`/`JoinSession`/`LeaveSession` as unrelated packages under `game/session/usecases/{createsession,joinsession,leavesession}/`, despite being transitions of one Session lifecycle, and introduced `game/session/internal/actors/` as a shared horizontal Actor/Participant persistence API consumed directly by Join and Leave to deduplicate code. The then-current `domain-logic-placement.md`/`repositories.md` did not clearly say whether this was correct, leaving:

1. no explicit Workflow-vs-Use-Case classification;
2. no statement that package organization should communicate behavior/lifecycle ownership;
3. no boundary between legitimate shared persistence infrastructure (locking, idempotency, error classification) and horizontal entity-centric repository APIs;
4. no guidance on when local persistence duplication is preferable to extraction;
5. no statement that repositories need not make application code hypothetically storage-engine-independent;
6. no statement that workflow grouping does not imply one big service/repository.

This was implementation produced under a standard that was ambiguous enough to permit the structure — not Codebase Agent non-compliance with a rule that did not yet exist clearly.

## Human Decision

HUMAN-APPROVED, 2026-09-09 (delivered directly as a Codebase Agent handoff, decisions given in full — not re-litigated here):

- the core organizing principle (behavior locality over entity/domain-wide-service organization);
- the Workflow vs. Use Case classification and its package-naming implication (`workflows/<name>/...` vs `usecases/<capability>/...`);
- no domain-wide God Service default, and no undifferentiated flat `usecases/` bucket either;
- repository contracts belong to behavior consumers even when method shapes coincide;
- repositories may know the persistence model/schema and are not required to hide it for hypothetical storage-engine replaceability;
- one repository operation may encapsulate multi-table persistence for one logical behavior;
- the sharing rule: centralize genuine cross-cutting protocols/mechanics (locking, idempotency, transaction mechanics, DB error classification); do not centralize horizontal entity-CRUD packages (`internal/actors`-style) merely to deduplicate;
- small local persistence duplication is preferable to premature extraction when the duplicated behaviors' semantics may diverge independently;
- private mechanical reuse inside one repository implementation remains fine as long as it doesn't become a cross-behavior API;
- no ceremonial mandatory Service->Repository->Datastore->GORM layering;
- enforcement stays code review only, no linter;
- migration strategy: FUTURE CODE ONLY effective immediately; MANDATORY TARGETED MIGRATION for the current Session Runtime implementation (must happen before Slice 2 continues, routed separately); OPPORTUNISTIC MIGRATION for all other existing code (no repository-wide rewrite authorized by this standard).

## What Was Persisted

- `docs/engineering/standards/domain-logic-placement.md` — added "Core Organizing Principle: Behavior Locality", extended Package Naming with `workflows/`/`usecases/` guidance, added the full "Workflow vs Use Case" section (with a "Classifying Ambiguous Cases" subsection), added "No Domain-Wide God Service", extended Repository Contracts with "Workflow Grouping Does Not Imply A Shared Repository Contract", and extended Enforcement's flagged-patterns list. Existing Preferred Placement Order, Type Ownership, and the `businessservice` OPPORTUNISTIC MIGRATION note were preserved unchanged.
- `docs/engineering/standards/repositories.md` — added "Repositories May Know The Persistence Model", "Multi-Table Persistence Encapsulation" (including the no-ceremonial-Datastore-layer rule), "Sharing Rule: Protocols/Mechanics, Not Horizontal Entity APIs" (with the divergence-legitimacy heuristic and the `internal/sessionlock`/`internal/idempotency`/`internal/pgerrs` vs. `internal/actors`-style worked contrast), "Prefer Semantic Independence Over Premature DRY", "Private Mechanical Reuse Remains Allowed", and an Enforcement section. Existing Repository Queries conventions were preserved unchanged.
- `docs/engineering/standards/INDEX.md` — widened the one-line descriptions for `repositories.md` and `domain-logic-placement.md` to mention the newly-owned concerns; no new standard file was created, no routing/ownership changed, no ARCHITECTURE.md change (no genuine architecture-level contradiction was found — this is an engineering convention implementing existing architecture, not a new bounded-context rule).

Cross-links use `[[name]]`-equivalent prose references (`repositories.md -> Sharing Rule`, `domain-logic-placement.md -> Repository Contracts`) rather than duplicating the canonical rule text in both files.

## Explicitly Not Done In This Handoff

- No refactor of `game/session/usecases/{createsession,joinsession,leavesession}/` or `game/session/internal/actors/` was performed.
- `docs/work/active/WORK-0001-session-lobby-foundation.md`'s status was not modified.
- No repository-wide migration of any other existing usecase/service/repository structure was performed or authorized.
- No new Session Runtime architecture ADR was created; no accepted Session Runtime architecture changed.
- No Slice 2 work was started.

## Dry-Run Validation (recorded for future reference, not new decisions)

- **A. Join and Leave both query SessionActor**: each behavior owns its persistence need; identical small query duplication is acceptable; do not auto-extract an `ActorRepository`. Consistent with `repositories.md -> Prefer Semantic Independence Over Premature DRY`.
- **B. Join/Leave/Start need identical Session mutation locking**: share one locking mechanism (`internal/sessionlock`-style); independent implementations would risk violating correctness. Consistent with `repositories.md -> Sharing Rule`.
- **C. Several operations use idempotency**: share the idempotency protocol/mechanics; each behavior still owns its own semantic request-equivalence fields/outcome handling. Consistent with `repositories.md -> Sharing Rule`.
- **D. Session lifecycle Create/Join/Leave/Start**: discoverable under one Session-lifecycle workflow ownership; remain separate focused services/packages; no mega lifecycle service, no single mega repository. Consistent with `domain-logic-placement.md -> Workflow vs Use Case` and `-> No Domain-Wide God Service`.
- **E. Independent Session query (e.g. list current Sessions for a User)**: may live under `usecases/` — an independent capability, not a lifecycle transition merely because it references Session state. Consistent with `domain-logic-placement.md -> Workflow vs Use Case -> Use Case`.
- **F. Repository operation spanning multiple tables (e.g. CreateSession)**: one behavior-oriented repository operation may encapsulate several inserts/updates; no mandatory Datastore layer; no row-by-row CRUD orchestration forced onto the application service. Consistent with `repositories.md -> Multi-Table Persistence Encapsulation`.

## Relationship To The Session Runtime Initiative

The mandatory targeted migration this standard requires for the current Session Runtime implementation is tracked as continuation state in `docs/ai/workspaces/active/session-runtime-v1/AI_CONTEXT.md`, not duplicated here. See that file for the concrete migration checklist (move Create/Join/Leave into a discoverable Session-lifecycle workflow organization while keeping focused per-step components; remove/refactor `game/session/internal/actors`; move its persistence needs into behavior-local Join/Leave repository implementations; retain the legitimate shared `sessionlock`/`idempotency`/`pgerrs` mechanisms) and the gating rule (this must happen before Slice 2 continues). That migration is routed through Feature Development / an implementation step against the `session-runtime-v1` initiative, not through this Engineering Standard workspace.
