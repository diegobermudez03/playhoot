# Engineering Radar

Status: NON-AUTHORITATIVE TECHNICAL RADAR

This is the canonical location for Playhoot's current engineering recommendations and future concerns, but the content is NON-AUTHORITATIVE.

A radar item is:

- an AI recommendation or concern;
- not an accepted decision;
- not an engineering standard;
- not architecture;
- not a roadmap commitment;
- not an approved WORK specification;
- not implementation authority.

NOW != READY

A NOW radar item does not authorize a Codebase Agent to implement anything.

## Authority Boundaries

If a radar recommendation later produces an accepted product decision, architecture/domain decision, or engineering standard, the accepted fact must live in its canonical owner.

The Radar must not become a duplicate source of accepted truth. After such a decision, remove the radar item if the concern has been fully resolved at the recommendation level, or update it to describe only the meaningful remaining concern.

A radar item must never directly authorize implementation. If a recommendation reaches the point where concrete implementation is approved, use `FEATURE_DEVELOPMENT` and the WORK specification system.

The Radar is not:

- a product backlog;
- an engineering task tracker;
- a sprint board;
- a product roadmap;
- a list of all future features;
- a list of all technical debt.

Only persist concerns or recommendations whose continued visibility is useful at the Principal Engineer level.

## Horizons

The horizon is primarily an action/attention recommendation.

It is not:

- a WORK status;
- a decision status;
- a formal incident severity;
- an implementation priority automatically accepted by the human.

Impact or risk can be explained inside an item when useful. Do not introduce a separate mandatory severity system here.

## NOW

The concern/opportunity exists under current conditions and deserves active follow-up now or alongside near-term work.

NOW does not itself mean:

- implementation is approved;
- an incident exists;
- architecture has been decided.

No items have yet been persisted under the Engineering Radar mechanism.

## SOON

The concern does not need immediate action but is likely worth addressing before a meaningful near-term milestone or before continued development makes it materially more expensive/risky.

No items have yet been persisted under the Engineering Radar mechanism.

## LATER

The concern is legitimate, but current conditions do not justify acting on it.

A LATER item should have a meaningful reevaluation trigger where possible.

No items have yet been persisted under the Engineering Radar mechanism.

## NOT NEEDED

The concern/technology/sophistication has been considered and is not justified under current conditions.

NOT NEEDED is not a permanent universal rejection. Include a reevaluation trigger where future conditions could change the assessment.

Persist NOT NEEDED items selectively. Do not fill the radar with every technology Playhoot could theoretically use.

A NOT NEEDED item is most useful when:

- the idea is likely to recur;
- recording why it is unnecessary helps prevent repeated architecture-astronautics;
- a future trigger for reconsideration is meaningful.

NOT NEEDED means "currently not justified", not "completed."

No items have yet been persisted under the Engineering Radar mechanism.

## Persisted Item Shape

Use descriptive headings rather than persistent IDs.

Expected shape:

### <Concern / Recommendation>

**Area**
<Observability / Reliability / Data / Security / etc.>

**Current state / evidence**
- <specific evidence from current implementation/canonical context>

**Risk / opportunity**
- <what could go wrong or what value is being missed>

**Recommendation**
- <concrete recommendation at the appropriate abstraction level>

**Why this horizon**
- <why NOW / SOON / LATER / NOT NEEDED>

**Reevaluate when**
- <trigger, when useful>
- None, only when genuinely no trigger is useful.

**Next process**
- <PRODUCT_DISCUSSION / ARCHITECTURE_DISCUSSION / DOMAIN_DESIGN / GUIDED_TECHNICAL_EXPLORATION / ENGINEERING_STANDARD / FEATURE_DEVELOPMENT / None>

**Last reviewed**
YYYY-MM-DD

Optional references may be included when useful:

- canonical path;
- ADR/PDR;
- engineering standard;
- WORK;
- implementation file.

Do not require empty boilerplate fields beyond those needed to make the recommendation understandable.

## Maintenance

The Radar is a living document. A later Principal Engineer Review may:

- add a meaningful new item;
- update evidence;
- update recommendation;
- move an item between horizons;
- change its reevaluation trigger;
- update Next process;
- remove an item that has been resolved or is no longer useful.

Do not create duplicate items for the same underlying concern merely because a new review occurred. Update the existing item.

Do not preserve obsolete items solely for history. Git history and completed WORK/decisions preserve historical information where appropriate.

When a concern is actually resolved, remove it from the current Radar unless there is still a meaningful remaining concern. Do not automatically move every resolved concern to NOT NEEDED.

Do not create a permanent resolved/archive section.

## Evidence and Scope

Radar recommendations must be grounded in current evidence from relevant canonical documentation, current-state domain/system documentation, code, tests, migrations, existing standards, or active work as appropriate.

Do not generate concerns merely from generic "best practices". For every material technology recommendation, apply the anti-overengineering questions from `docs/ai/OPERATING_MODEL.md`.

A Principal Engineer Review may be broad/system-wide or intentionally focused on a technical area.

When a review is scoped, do not silently reclassify unrelated existing Radar items as though they were reevaluated.

Only update:

- items actually reconsidered by the current review;
- newly discovered items in scope;
- items directly invalidated/resolved by evidence inspected in the review.

Absence of an item does not mean:

- the area was comprehensively reviewed;
- the area is guaranteed safe;
- the area has been permanently declared adequate.

Do not persist every adequate area as a Radar item. The Radar is not a compliance matrix.

## Next Process Routing

Radar recommendations should route to the process needed to reason about the next material step.

Typical routing:

- Product question -> `PRODUCT_DISCUSSION`
- Architecture/system design -> `ARCHITECTURE_DISCUSSION`
- Domain ownership/boundary -> `DOMAIN_DESIGN`
- Technical area that needs understanding/exploration -> `GUIDED_TECHNICAL_EXPLORATION`
- Reusable engineering rule -> `ENGINEERING_STANDARD`
- Concrete already-decided implementation -> `FEATURE_DEVELOPMENT`
- No currently justified follow-up -> `None`

Choosing a Next process does not automatically start that process.

## AI and Human Authority

Because Radar content is explicitly NON-AUTHORITATIVE, a Conversational AI may recommend radar items to add, update, move, or remove as part of a Principal Engineer Review, without a human-approval ceremony and without converting them into human-approved decisions. Persisting those changes to this file is a CODEBASE AGENT step, via a CODEBASE AGENT HANDOFF from the Conversational AI.

This does not grant authority to:

- accept architecture;
- accept product behavior;
- establish an engineering standard;
- approve WORK;
- implement the recommendation.

The human decides which recommendations deserve follow-up and remains final authority for material decisions.

The human may also challenge or override radar classifications.
