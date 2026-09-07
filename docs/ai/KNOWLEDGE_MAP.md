# Knowledge Map

Status: CANONICAL ROUTING MAP

This is a machine-facing routing document. It does not explain Playhoot. It tells AI agents where to retrieve authoritative context.

## Routing Rules

- Do not scan all Markdown files.
- Load only task-relevant context.
- Code, tests, and migrations are authoritative for actual implemented behavior.
- Accepted canonical documents are authoritative for accepted design.

## Routing Table

| Need | Canonical | Status | Current fallback |
| --- | --- | --- | --- |
| Product vision | `PROJECT_OVERVIEW.md` | AVAILABLE | |
| Current release/product scope | `docs/product/PRODUCT_STATE.md` | AVAILABLE | |
| Product roadmap | `docs/product/ROADMAP.md` | AVAILABLE | |
| Non-authoritative product ideas | `docs/product/IDEAS.md` | AVAILABLE / NON-AUTHORITATIVE | |
| Global architecture | `ARCHITECTURE.md` | AVAILABLE | |
| Current system visualization | `docs/architecture/SYSTEM_MAP.md` | AVAILABLE | |
| Current cross-domain flows | `docs/architecture/CROSS_DOMAIN_FLOWS.md` | AVAILABLE | |
| Domain responsibility | `<domain>/README.md` | DOMAIN-DEPENDENT | |
| Domain implementation state | `<domain>/CURRENT_STATE.md` | DOMAIN-DEPENDENT | |
| Domain DB schema | `<domain>/docs/DATA_MODEL.md` | DOMAIN-DEPENDENT | migrations plus persistence code |
| Domain current flows | `<domain>/docs/FLOWS.md` | DOMAIN-DEPENDENT | implementation plus tests |
| Domain documentation template | `docs/ai/templates/domain/` | AVAILABLE | |
| Engineering standards | `docs/engineering/standards/INDEX.md` and referenced standards | AVAILABLE | |
| Engineering recommendations / future concerns | `docs/engineering/ENGINEERING_RADAR.md` | AVAILABLE / NON-AUTHORITATIVE | |
| Architecture decision rationale | `docs/decisions/architecture/INDEX.md` and referenced ADRs | AVAILABLE | |
| Product decision rationale | `docs/decisions/product/INDEX.md` and referenced PDRs | AVAILABLE | |
| Decision-record process/templates | `docs/decisions/README.md` and `docs/decisions/templates/` | AVAILABLE | |
| Current implementation work | `docs/work/active/` | AVAILABLE | |
| Completed implementation history | `docs/work/completed/` | AVAILABLE | |
| Work specification process/template | `docs/work/README.md` and `docs/work/templates/` | AVAILABLE | |
| AI development behavior | `docs/ai/OPERATING_MODEL.md` | AVAILABLE | |
| Execution surface / actor authority | `docs/ai/OPERATING_MODEL.md` | AVAILABLE | |
| AI workflow evolution history | `docs/ai/CHANGELOG.md` | AVAILABLE / HISTORICAL | |
| Implementation / independent review protocol | `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` | AVAILABLE | |
| Agent execution protocol | `docs/ai/protocols/*` | AVAILABLE / AGENT-FACING | |
| Natural request routing / process & initiative selection | `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md` | AVAILABLE / AGENT-FACING | |
| Human AI workflow entry point | `docs/ai/README.md` | AVAILABLE | |
| Human process/runbook | `docs/ai/processes/*` | AVAILABLE / HUMAN-FACING | |
| Workspace mechanics | `docs/ai/workspaces/README.md` | AVAILABLE | |
| Temporary active process/initiative workspace, open-process discovery | `docs/ai/workspaces/active/<initiative>/` | TEMPORARY / NON-AUTHORITATIVE | |

`DATA_MODEL.md` diagrams, once created, must represent current implementation, show all persisted columns, and show exact column-to-column relationships. Indexes and SQL types are not required.

## Accepted Domain Documentation

DOMAIN-DEPENDENT entries apply only to accepted domains. They do not mean every top-level directory is an accepted domain.

| Domain | Model | Current State | Data Model | Flows |
| --- | --- | --- | --- | --- |
| Game | `game/README.md` | `game/CURRENT_STATE.md` | `game/docs/DATA_MODEL.md` | `game/docs/FLOWS.md` |
| Identity | `identity/README.md` | `identity/CURRENT_STATE.md` |  |  |

## Specialized Documentation

Package-local contracts and implementation documentation may remain near the implementation. Examples currently present in the repository include:

- `game/language/v1/engine/LOGICAL_CONTRACT.md` -> engine semantics/invariants.
- `game/language/v1/engine/README.md` -> engine usage.
- `game/language/v1/engine/IMPLEMENTATION.md` -> engine implementation/maintenance context.
- `game/language/v1/program/*` -> game-language/program-specific context.

The following files are game-generation product AI artifacts, not instructions for AI agents developing Playhoot:

- `game/language/v1/program/DEFINITION.md`
- `game/language/v1/program/GAME_BRIEF.md`

## Domain Context Resolution

When working in a domain, load:

1. global context required by the task;
2. domain `README.md` if available;
3. domain `CURRENT_STATE.md` if available;
4. relevant `DATA_MODEL.md` or `FLOWS.md` if available;
5. relevant specialized package docs;
6. actual code, tests, and migrations needed for the question.

Do not load unrelated domains unless the task crosses those boundaries.
