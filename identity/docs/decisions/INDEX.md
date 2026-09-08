# Identity Architecture Decisions

Status: DECISION INDEX (DOMAIN-SCOPED)

These records preserve the historical architecture rationale for the Identity bounded context. They are not current accepted truth.

Current accepted truth for Identity is owned by `identity/README.md` and `identity/CURRENT_STATE.md`. Agents should not read every Identity ADR by default; load an individual record only when the rationale/history behind a current Identity rule is actually needed.

This index is the Identity decision family under the repository-wide routing model described in `docs/decisions/README.md` and `docs/decisions/INDEX.md`. Naming: `IDENTITY-ADR-NNNN`, with an independent sequence from the global architecture family and from other domains.

| ID | Title | Status | Created | Legacy ID | Canonical Impact |
| --- | --- | --- | --- | --- | --- |
| [IDENTITY-ADR-0001](IDENTITY-ADR-0001-identity-user-public-identity-boundary.md) | Identity User Public Identity Boundary | ACCEPTED | 2026-09-06 | ADR-0006 | `ARCHITECTURE.md`, `identity/README.md`, `identity/CURRENT_STATE.md`, `game/README.md`, `docs/ai/KNOWLEDGE_MAP.md`, `docs/engineering/standards/cross-domain-reference-naming.md` |

Next Identity ADR: `IDENTITY-ADR-0002`.

This ID was migrated from the previously centralized global architecture family (`docs/decisions/architecture/`) on 2026-09-07. Historical status, rationale, alternatives, consequences, and created date were preserved unchanged; only the identifier/location/cross-reference wiring changed. See `docs/decisions/LEGACY_ADR_ID_MAP.md` for the full old-ID -> new-ID mapping.

Whenever an Identity ADR is created or its lifecycle status changes, update this index.
