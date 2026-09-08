# Domain-Scoped ADR Ownership/Routing Migration

Process: AI Workflow Change

Status: RESOLVED - approved and applied 2026-09-07. No checkpoint is currently pending.

## Current Workflow Problem

All architecture decision records lived under one centralized directory, `docs/decisions/architecture/`, regardless of whether a decision's authority was global/cross-domain or contained entirely within one bounded context (Game, Identity). As Game/Session Runtime architecture discussion produced many ADRs (ADR-0002 through ADR-0004, ADR-0006 through ADR-0013), the centralized directory mixed genuinely global rules (ADR-0001, ADR-0005) with decisions that were really one domain's own model of itself, making it harder to discover "how did Game's architecture evolve" without reading unrelated Identity/global records, and harder to keep the directory meaningful as it grows.

## Current Behavior And Owner

Before this change: `docs/decisions/README.md` defined a single ADR family at `docs/decisions/architecture/` with identifier `ADR-NNNN` and one sequence, regardless of scope. `docs/decisions/architecture/INDEX.md` listed all 13 existing ADRs together.

## Approved Scope-Based Model

- Global/cross-domain ADRs remain centralized at `docs/decisions/architecture/`, identifier `ADR-NNNN`.
- Bounded-context/domain-scoped ADRs move to `<domain>/docs/decisions/`, identifier `<DOMAIN>-ADR-NNNN`, each domain with its own independent sequence and `INDEX.md`.
- Scope resolution: a decision whose authority is contained within one bounded context (even if another domain consumes the resulting public contract) is domain-local; a decision that itself establishes a rule/contract spanning bounded contexts is global.
- A root registry (`docs/decisions/INDEX.md`) routes to each family's index without duplicating records.
- Existing accepted ADRs were migrated now (MIGRATE EXISTING ADR ARTIFACTS strategy, not FUTURE ARTIFACTS ONLY), because the ADR set was still small enough to migrate cheaply before the centralized family grew further.
- Migrated domain ADRs were renamed to namespaced identifiers and keep a `Legacy ID:` metadata line; a durable mapping (`docs/decisions/LEGACY_ADR_ID_MAP.md`) resolves every legacy identifier to its new canonical ID/location.
- The global family's historical high-water mark (through legacy `ADR-0013`) is preserved: the next newly allocated global ADR must be `ADR-0014` or later, never reusing a migrated legacy number.
- No ADR status, rationale, alternatives, consequences, or created date was changed. No product/architecture/domain truth changed - only where decision rationale is filed and how it is identified.

## Migration Map (Old -> New)

| Legacy ID | New Canonical ID | New Location |
| --- | --- | --- |
| ADR-0001 | ADR-0001 (unchanged) | `docs/decisions/architecture/` |
| ADR-0002 | GAME-ADR-0001 | `game/docs/decisions/` |
| ADR-0003 | GAME-ADR-0002 | `game/docs/decisions/` |
| ADR-0004 | GAME-ADR-0003 | `game/docs/decisions/` |
| ADR-0005 | ADR-0005 (unchanged) | `docs/decisions/architecture/` |
| ADR-0006 | IDENTITY-ADR-0001 | `identity/docs/decisions/` |
| ADR-0007 | GAME-ADR-0004 | `game/docs/decisions/` |
| ADR-0008 | GAME-ADR-0005 | `game/docs/decisions/` |
| ADR-0009 | GAME-ADR-0006 | `game/docs/decisions/` |
| ADR-0010 | GAME-ADR-0007 | `game/docs/decisions/` |
| ADR-0011 | GAME-ADR-0008 | `game/docs/decisions/` |
| ADR-0012 | GAME-ADR-0009 | `game/docs/decisions/` |
| ADR-0013 | GAME-ADR-0010 | `game/docs/decisions/` |

Full detail: `docs/decisions/LEGACY_ADR_ID_MAP.md`.

## Affected Artifacts

- `docs/decisions/README.md` (scope-based routing model, numbering, historical high-water mark, legacy-migration exception to Historical Immutability).
- `docs/decisions/INDEX.md` (new root decision-family registry).
- `docs/decisions/LEGACY_ADR_ID_MAP.md` (new legacy ID map).
- `docs/decisions/architecture/INDEX.md` (now global-only; migration/high-water-mark notes).
- `docs/decisions/templates/ARCHITECTURE_DECISION.template.md` (one template covers both `ADR-NNNN` and `<DOMAIN>-ADR-NNNN`).
- `game/docs/decisions/` (new: 10 migrated GAME-ADRs plus `INDEX.md`).
- `identity/docs/decisions/` (new: 1 migrated IDENTITY-ADR plus `INDEX.md`).
- `ARCHITECTURE.md`, `game/README.md`, `identity/README.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/language/v1/engine/README.md`, `game/language/v1/program/README.md` (updated ADR path/identifier references).
- `docs/ai/KNOWLEDGE_MAP.md` (routing split by family; decision-history vs current-truth guidance).
- `docs/ai/protocols/ARCHITECTURE_DISCUSSION.md`, `docs/ai/protocols/DOMAIN_DESIGN.md` (scope-resolution step before ADR ID allocation).
- `docs/ai/workspaces/active/session-runtime-v1/HUMAN_REVIEW.md`, `AI_CONTEXT.md` (reference updates only; Session Runtime architecture content unchanged).
- `docs/ai/CHANGELOG.md` (applied entry).

## Compatibility / Migration Strategy

MIGRATE EXISTING ADR ARTIFACTS. All 11 domain-scoped legacy ADRs were moved and renamed; the 2 genuinely global ADRs (ADR-0001, ADR-0005) stayed in place. No duplicate authoritative bodies remain under the old locations. Future ADRs use the scope-based model from creation.

## Validation Results

- Every migrated record represented exactly once canonically (old files deleted after content moved).
- Every legacy ID has exactly one mapping in `docs/decisions/LEGACY_ADR_ID_MAP.md`.
- Both new domain indexes point only to files that exist.
- The global index (`docs/decisions/architecture/INDEX.md`) contains only ADR-0001 and ADR-0005.
- Repository-wide search for `docs/decisions/architecture/ADR-000[2346789]`/`ADR-001[0-3]` returns no remaining stale references outside this workspace's own historical/legacy notes.
- No ADR status, rationale, alternatives, consequences, or created date was substantively changed.
- No product/architecture/domain/code/test/migration truth changed.

## What Happens If Approved

This describes work already applied under explicit human authorization (see the originating request). No further approval step is pending; this file is retained as the durable record of the applied change per `docs/ai/protocols/AI_WORKFLOW_CHANGE.md`.
