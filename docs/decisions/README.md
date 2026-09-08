# Decision Records

Status: CANONICAL PROCESS REFERENCE

Decision records preserve important historical decisions and their rationale. They do not replace canonical product, architecture, domain, implementation, or work-specification documents.

## Ownership

- Decision record: historical decision plus rationale.
- Canonical knowledge: current accepted truth.
- Implementation/current-state docs: what actually exists.
- Work specification: approved implementation change.

```text
Discussion
    |
Human decision
    |
Decision record -------- why?
    |
Canonical knowledge ----- what is accepted now?
    |
Work specification ------ what should implementation change?
    |
Implementation ---------- what actually exists?
```

Not every small change needs every stage.

## Decision Families

Architecture Decision Records (ADRs) preserve architecture decision rationale. Use for significant system architecture, bounded-context/domain, ownership, cross-domain communication, persistent-data semantics, consistency, concurrency, infrastructure/deployment, major dependency, or durable public technical-contract decisions. Do not use ADRs for trivial implementation details.

Architecture decision-record ownership follows architectural scope, not one centralized directory. There are two kinds of ADR family:

### Global / cross-domain ADRs

- Location: `docs/decisions/architecture/`
- Filename: `ADR-NNNN-short-kebab-title.md`
- Use when the decision is system-wide or crosses bounded-context ownership: system-wide architecture principles, cross-domain communication rules, global persistence/reference rules, infrastructure/deployment decisions not owned by one bounded context, coordination contracts whose authority genuinely spans multiple bounded contexts, or global technical policies.

### Domain-scoped ADRs

- Location: `<domain>/docs/decisions/` (for example `game/docs/decisions/`, `identity/docs/decisions/`).
- Filename: `<DOMAIN>-ADR-NNNN-short-kebab-title.md`.
- Identifier: `<DOMAIN>-ADR-NNNN` (for example `GAME-ADR-0001`, `IDENTITY-ADR-0001`). The prefix must be stable, unambiguous, and derived from the bounded-context identity, not from an arbitrary package/subpackage name.
- Use when the decision's architectural authority/rationale is contained within one bounded context, even if the resulting public contract is consumed by another domain. The fact that another domain consumes a public capability does not by itself make the producer's decision global.
- Each domain decision family has its own independent sequence and its own `INDEX.md`.

### Scope resolution

Before allocating an ADR ID, determine scope:

- **Domain-local** if the decision primarily answers "how does bounded context X own/model/execute this?" and no other bounded context shares authority over the decision (for example: Session Runtime turn semantics, Game Language initialization semantics, session lifecycle, Identity's definition of its exported `User` entity, internal capability boundaries inside one bounded context).
- **Global / cross-domain** if the decision itself establishes a rule/contract whose authority spans bounded contexts or the whole system (for example: cross-domain references never imply DB foreign keys, general cross-domain UUID semantics, system-wide transaction/communication policy, platform-wide orchestration ownership).
- **Ambiguous scope**: identify the canonical current-truth owner and who can change the decision independently; prefer local ownership when one bounded context clearly has decision authority; use global only when authority genuinely spans contexts.

Do not duplicate the same decision in more than one family. A consuming domain links to the authoritative record instead of re-recording it.

The root decision-family registry at `docs/decisions/INDEX.md` routes to each family's index; it does not duplicate individual records.

Product Decision Records (PDRs) preserve product decision rationale.

- Location: `docs/decisions/product/`
- Filename: `PDR-NNNN-short-kebab-title.md`
- Use for significant durable product behavior, release scope, target-user/use-case, public product experience, capability inclusion/exclusion, monetization, or product-policy decisions.
- Do not use PDRs for every UI copy/detail or casual idea.
- PDR routing is unchanged by the ADR scope-based model above.

## Numbering

Each ADR family (global, and each domain) and Product use independent sequences.

- Global architecture: `ADR-0001`, `ADR-0002`, ...
- Each domain's ADRs: `<DOMAIN>-ADR-0001`, `<DOMAIN>-ADR-0002`, ... (its own sequence, starting at `0001` independently of every other family).
- Product: `PDR-0001`, `PDR-0002`, ...

Determine the next number from the existing records/index for that specific family — a domain family's next number is never derived from the global family's count, or vice versa.

### Historical high-water mark

Playhoot's ADRs were originally centralized under `docs/decisions/architecture/` before scope-based domain families existed. When those records were migrated to their owning domain family, their old `ADR-NNNN` identifiers became historical/legacy — see `docs/decisions/LEGACY_ADR_ID_MAP.md`. A newly allocated **global** ADR must never reuse a legacy `ADR-NNNN` identifier, including one that migrated to a domain family; use the recorded high-water mark plus one (or a later existing global ADR, whichever is greater) rather than filling an apparent gap. Domain families created after that migration are unaffected by the legacy high-water mark; they simply start their own sequence at `0001`.

Never renumber historical records because another record was rejected, superseded, or deprecated. Filenames should remain stable after creation except for an exceptional repository-maintenance reason (such as the scope-based migration itself).

## When To Create A Record

A decision record is not mandatory for every discussion.

Persist rationale when a decision is important enough that a future engineer or AI is likely to ask: "Why did Playhoot choose this?"

Useful signals include:

- alternatives were materially plausible;
- tradeoffs are non-obvious;
- the decision has durable consequences;
- reversing it would be expensive;
- the rejected alternative is likely to be proposed again;
- understanding the reason materially helps future evolution.

A discussion that produces no durable decision may require no record.

A decision reached completely in one discussion may be created directly as ACCEPTED or REJECTED. It does not have to first exist in the repository as PROPOSED.

Use PROPOSED when the proposal itself needs to survive across discussions, reviews, or time before the human decision is final.

## Status Model

The only current statuses are:

- PROPOSED
- ACCEPTED
- REJECTED
- SUPERSEDED
- DEPRECATED

PROPOSED:

- Not accepted.
- Has no architecture/product authority.
- May evolve while under discussion.

ACCEPTED:

- Explicitly approved by the human decision maker.
- Preserves rationale.
- Resulting accepted facts must also be reflected in their canonical knowledge owners.

REJECTED:

- Explicitly considered and not adopted.
- Does not change canonical product/architecture.
- Preserved only when its rationale has future value.

SUPERSEDED:

- Was previously ACCEPTED.
- A specific newer decision record replaces it.
- The old record remains historical.
- Link old and new records in both directions.

DEPRECATED:

- Was previously ACCEPTED.
- The decision intentionally no longer applies.
- There is no single direct replacement decision.
- Canonical knowledge must be updated to remove/change the old accepted fact.

Normal lifecycle:

- PROPOSED -> ACCEPTED
- PROPOSED -> REJECTED
- ACCEPTED -> SUPERSEDED
- ACCEPTED -> DEPRECATED

REJECTED, SUPERSEDED, and DEPRECATED are normally terminal.

If a rejected idea is reconsidered materially later, create a new decision record rather than rewriting history.

Do not introduce DEFERRED as another status. Deferred concerns should normally live in the appropriate non-authoritative place or remain unpersisted.

## Historical Immutability

While a record is PROPOSED, its proposal/rationale may evolve.

Once ACCEPTED or REJECTED:

- do not substantively rewrite its historical rationale;
- typo/link/format fixes are allowed when they do not change meaning.

If an accepted decision later changes, create a new decision record. Then:

- mark the previous record SUPERSEDED when the new decision directly replaces it;
- link the previous record to the new record;
- link the new record to the previous record;
- update canonical knowledge.

Do not edit the old decision text to make it appear as though the new decision had always been the decision.

### Exceptional repository-maintenance migration

An exceptional repository-maintenance operation (such as the ADR ownership/routing migration to scope-based families) may move/rename a record and update its identifier and internal cross-references without changing its status, rationale, alternatives, consequences, or created date. A migrated record keeps a `Legacy ID:` metadata line (alongside `Status`/`Created`/`Supersedes`) recording its prior identifier, and the move is recorded in a durable mapping (`docs/decisions/LEGACY_ADR_ID_MAP.md`) so historical references remain resolvable. This is not the normal historical-immutability path and must not be used to reverse, reinterpret, or reword an accepted/rejected decision.

## One Fact, One Owner

Decision records and canonical knowledge have different ownership.

Decision records own historical context, alternatives considered, decision/outcome at that point in time, rationale/tradeoffs, and consequences known when decided.

Canonical documents own current accepted product/architecture/domain facts.

An ADR/PDR may contain a concise snapshot of the decision necessary to understand its rationale, but future agents must not use the decision-record directory as a substitute for canonical current-state knowledge.

## Accepted Decision Synchronization

When a decision becomes ACCEPTED:

1. Persist its rationale when the decision meets the persistence threshold.
2. Identify the canonical knowledge owner(s) affected.
3. Update those canonical documents in the same documentation change whenever practical.
4. Identify implementation impact separately.

Acceptance does not mean implementation exists. Do not modify current-state diagrams to show unimplemented behavior as implemented.

If implementation work is required, route it to `docs/ai/processes/FEATURE_DEVELOPMENT.md` and the persistent work specification system in `docs/work/README.md`. Concrete approved work lives under `docs/work/active/`.

## Decision / Canonical Drift

Decision records are not the primary current-truth source.

If an ACCEPTED decision appears inconsistent with its expected current canonical owner, report:

```text
DECISION / CANONICAL DRIFT

Decision record:
...

Canonical owner:
...

Conflict:
...

Likely explanation:
...

Recommended resolution:
...
```

A SUPERSEDED/DEPRECATED record differing from current canonical knowledge is normally expected historical evolution, not drift.

## Template Guidance

The templates under `docs/decisions/templates/` contain authoring guidance in addition to the shape of the resulting record.

When instantiating an ADR or PDR, do not copy template-only instructions, examples, or guidance into the final decision record. Final records should contain only actual decision information.
