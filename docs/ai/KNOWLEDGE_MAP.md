# Knowledge Map

Status: TRANSITIONAL

This is a machine-facing routing document. It does not explain Playhoot. It tells AI agents where to retrieve authoritative context.

## Routing Rules

- Do not scan all Markdown files.
- Load only task-relevant context.
- Code, tests, and migrations are authoritative for actual implemented behavior.
- Accepted canonical documents are authoritative for accepted design.
- If a target canonical document is marked PLANNED and does not exist yet, use the listed current fallback and inspect code where appropriate.
- Never interpret PLANNED documents as if they already existed.

## Routing Table

| Need | Canonical | Status | Current fallback |
| --- | --- | --- | --- |
| Product vision | `PROJECT_OVERVIEW.md` | AVAILABLE | |
| Current release/product scope | `docs/product/PRODUCT_STATE.md` | PLANNED | `PROJECT_OVERVIEW.md` plus current task context |
| Product roadmap | `docs/product/ROADMAP.md` | PLANNED | |
| Non-authoritative product ideas | `docs/product/IDEAS.md` | PLANNED | |
| Global architecture | `ARCHITECTURE.md` | AVAILABLE / NEEDS NORMALIZATION | |
| Current system visualization | `docs/architecture/SYSTEM_MAP.md` | PLANNED | `ARCHITECTURE.md` plus repository inspection |
| Current cross-domain flows | `docs/architecture/CROSS_DOMAIN_FLOWS.md` | PLANNED | architecture docs plus repository inspection |
| Domain responsibility | `<domain>/README.md` | PARTIAL / DOMAIN-DEPENDENT | |
| Domain implementation state | `<domain>/CURRENT_STATE.md` | PLANNED | |
| Domain DB schema | `<domain>/docs/DATA_MODEL.md` | PLANNED | migrations plus persistence code |
| Domain current flows | `<domain>/docs/FLOWS.md` | PLANNED | implementation plus tests |
| Engineering standards | `docs/engineering/standards/INDEX.md` and referenced standards | PLANNED | `AGENTS.md` "Project Patterns" |
| Engineering recommendations / future concerns | `docs/engineering/ENGINEERING_RADAR.md` | PLANNED / NON-AUTHORITATIVE | |
| Architecture decision rationale | `docs/decisions/architecture/*` | PLANNED | |
| Product decision rationale | `docs/decisions/product/*` | PLANNED | |
| Current implementation work | `docs/work/active/*` | PLANNED | |
| Completed implementation history | `docs/work/completed/*` | PLANNED | |
| AI development behavior | `docs/ai/OPERATING_MODEL.md` | AVAILABLE | |
| AI workflow/runbook | `docs/ai/processes/*` | PLANNED | |

`DATA_MODEL.md` diagrams, once created, must show table columns and column-to-column relationships, but do not need indexes or SQL types.

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

## Transitional State

This map will be updated as later normalization steps create canonical documents. Missing planned documents are expected during migration and are not themselves errors.
