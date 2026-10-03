Process: AI Workflow Change
Topic: Domain-scoped ADR ownership/routing migration
Current stage: RESOLVED - applied
Current execution surface: CODEBASE AGENT
Related durable artifacts: `docs/decisions/README.md`, `docs/decisions/INDEX.md`, `docs/decisions/LEGACY_ADR_ID_MAP.md`, `docs/decisions/architecture/INDEX.md`, `docs/decisions/templates/ARCHITECTURE_DECISION.template.md`, `game/docs/decisions/INDEX.md`, `identity/docs/decisions/INDEX.md`, `docs/ai/KNOWLEDGE_MAP.md`, `docs/ai/protocols/ARCHITECTURE_DISCUSSION.md`, `docs/ai/protocols/DOMAIN_DESIGN.md`, `docs/ai/CHANGELOG.md`
Blocked by: None
Next action: None - migration complete. This workspace may be removed once the human has acknowledged the applied change (no other downstream work is tracked here).
Last durable checkpoint: Migrated all domain-scoped legacy ADRs (ADR-0002/0003/0004/0007/0008/0009/0010/0011/0012/0013) to `game/docs/decisions/GAME-ADR-0001..0010`, and ADR-0006 to `identity/docs/decisions/IDENTITY-ADR-0001`; kept ADR-0001/ADR-0005 global; created domain indexes, root registry, and legacy ID map; updated all repository-wide references; appended CHANGELOG entry.
Last updated: 2026-09-07

# Resume Context

This workspace records the applied AI Workflow Change that replaced the single centralized `docs/decisions/architecture/` ADR family with a scope-based model: global/cross-domain ADRs stay centralized; bounded-context-scoped ADRs move to `<domain>/docs/decisions/` with a namespaced `<DOMAIN>-ADR-NNNN` identifier and an independent per-domain sequence.

## What Was Approved

The human explicitly approved, in one request:

1. Changing ADR ownership/routing from centralized to scope-based decision families.
2. Keeping globally-scoped/cross-domain ADRs centralized.
3. Placing bounded-context/domain-scoped ADRs alongside their owning domain.
4. Migrating existing accepted ADRs now (not FUTURE ARTIFACTS ONLY).
5. Renaming migrated domain ADRs to namespaced identifiers.
6. Updating all repository references as part of the migration.

This is an ADR organization/routing change; it does not change what Playhoot has decided (status/rationale/dates unchanged) and does not reduce the ADR persistence threshold.

## Scope Resolution Applied

- ADR-0001 (Intra-Domain Responsibility Boundary) and ADR-0005 (Cross-Domain Public Entity References) stayed global: both establish system-wide rules that apply across every bounded context, not one domain's own model.
- ADR-0002, ADR-0003, ADR-0004, ADR-0007, ADR-0008, ADR-0009, ADR-0010, ADR-0011, ADR-0012, and ADR-0013 all concern Game/Session Runtime's own ownership/model/execution decisions (persistence boundary, durable-state boundary, actor/lifecycle model, lobby contract, identity translation, root roster contract, turn/persistence model, timer-recovery tradeoff, archival direction, disconnect/reconnect boundary) - moved to `game/docs/decisions/` as GAME-ADR-0001 through GAME-ADR-0010, in the same order.
- ADR-0006 (Identity User Public Identity Boundary) is Identity's own definition of its exported `User`/`UserUUID` concept, even though Game/Session Runtime consumes it - moved to `identity/docs/decisions/` as IDENTITY-ADR-0001.

## Cross-Reference Rewiring

Each migrated ADR's own body was checked for references to other ADRs by number and rewired to the new canonical ID of whichever family that reference now belongs to (a reference to another Game decision became `GAME-ADR-000X`; a reference to the still-global ADR-0005 stayed `ADR-0005`; a reference to the now-Identity ADR-0006 became `IDENTITY-ADR-0001`). No other text in any ADR was changed. Full old-ID -> new-ID -> new-location mapping: `docs/decisions/LEGACY_ADR_ID_MAP.md`.

## Numbering After Migration

- Global architecture: next allocation is `ADR-0014` (historical high-water mark ADR-0013 preserved; ADR-0001/ADR-0005 remain in the directory but do not change the high-water mark since it already accounts for every legacy ID through ADR-0013).
- Game: next allocation is `GAME-ADR-0011`.
- Identity: next allocation is `IDENTITY-ADR-0002`.

## Repository-Wide Reference Updates Applied

`ARCHITECTURE.md`, `game/README.md`, `identity/README.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/language/v1/engine/README.md`, `game/language/v1/program/README.md`, `docs/ai/KNOWLEDGE_MAP.md`, `docs/ai/workspaces/active/session-runtime-v1/HUMAN_REVIEW.md`, `docs/ai/workspaces/active/session-runtime-v1/AI_CONTEXT.md`, `docs/ai/protocols/ARCHITECTURE_DISCUSSION.md`, `docs/ai/protocols/DOMAIN_DESIGN.md`. Verified by repository-wide search that no stale `docs/decisions/architecture/ADR-000X` path reference remains for a migrated ID outside of intentional legacy/historical notes (this workspace and `docs/decisions/LEGACY_ADR_ID_MAP.md`).

## Dry Runs Performed (Conceptual)

- Game-only decision (new internal concurrency policy): current truth stays in `game/README.md`; if record-worthy, next ID is `GAME-ADR-0011`; global directory untouched. Coherent.
- Identity-only decision (User reconciliation semantics): current truth in `identity/README.md`; if record-worthy, next ID is `IDENTITY-ADR-0002`. Coherent.
- Cross-domain rule (all bounded contexts must use a particular message-ownership rule): global `ARCHITECTURE.md` owner; next global ID `ADR-0014`. Coherent.
- Domain public contract consumed elsewhere (Identity defines a new public entity Game consumes): stays an Identity ADR; Game links to it rather than duplicating. Coherent with the existing IDENTITY-ADR-0001 precedent.
- Evolution lookup ("why does Session Runtime restart Game Language timer delay after process restart") routes to `game/docs/decisions/INDEX.md` -> `GAME-ADR-0008`, no Identity/global scan needed. Coherent.
- Current-truth lookup ("how do Session timers work now") routes to `game/README.md` first; ADR history optional. Coherent.
- Legacy reference ("ADR-0011" in old text) resolves unambiguously via `docs/decisions/LEGACY_ADR_ID_MAP.md` to `GAME-ADR-0008`. Coherent.
- New global ADR allocation: must use `ADR-0014`, never a legacy number. Coherent.

## Explicitly Not Done

No product, architecture, domain, or engineering-standard truth was changed. No ADR status/rationale/alternatives/consequences/created-date was rewritten. No code/tests/migrations were touched. No new product/domain decision was created. Product Decision Record (PDR) routing/location was not changed.
