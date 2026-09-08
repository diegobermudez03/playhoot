# Decision Records Registry

Status: DECISION FAMILY ROUTING INDEX

This is a lightweight root registry. It routes to decision families; it does not duplicate individual ADRs/PDRs. See `docs/decisions/README.md` for the full process (scope resolution, naming, numbering, status model, historical immutability).

## Architecture Decision Families

Architecture decision-record ownership follows architectural scope: a decision whose authority spans bounded contexts or the whole system is global; a decision whose authority is contained within one bounded context is that domain's, even when its resulting public contract is consumed elsewhere.

| Scope | Family | Prefix | Index |
| --- | --- | --- | --- |
| Global / cross-domain | Architecture | `ADR-NNNN` | `docs/decisions/architecture/INDEX.md` |
| Game (Game Management, Session Runtime, Game Language) | Game | `GAME-ADR-NNNN` | `game/docs/decisions/INDEX.md` |
| Identity | Identity | `IDENTITY-ADR-NNNN` | `identity/docs/decisions/INDEX.md` |

Each family has its own independent numbering sequence and its own `INDEX.md`. A future accepted bounded context gets its own family the first time it needs a local ADR: create `<domain>/docs/decisions/`, an `INDEX.md`, a stable uppercase prefix, and start that domain's sequence at `0001` — do not pre-create empty decision directories for hypothetical domains.

Do not duplicate the same decision in more than one family. A consuming domain links to the authoritative record instead.

## Legacy Migration

Existing accepted ADRs were migrated from the previously centralized `docs/decisions/architecture/` directory to this scope-based model on 2026-09-07. `docs/decisions/LEGACY_ADR_ID_MAP.md` resolves every legacy `ADR-NNNN` identifier to its current canonical ID and location. The global family's next allocation must not reuse any legacy ID recorded there — see that file for the current high-water mark rule.

## Product Decision Records

Product Decision Record (PDR) routing is unchanged by this registry.

| Family | Prefix | Index |
| --- | --- | --- |
| Product | `PDR-NNNN` | `docs/decisions/product/INDEX.md` |

## Templates

`docs/decisions/templates/` holds the shared authoring templates for both `ADR-NNNN` and `<DOMAIN>-ADR-NNNN` records, and the Product Decision Record template.
