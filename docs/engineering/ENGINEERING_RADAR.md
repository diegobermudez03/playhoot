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

### CORS on the production bucket for game frontend packages

**Area**
Infrastructure / Web client integration

**Current state / evidence**
- `session_started` gives clients a signed GCS URL to the game's frontend package (`play/session/usecases/frontendaccess`).
- The web frontend's GameHost downloads it with `fetch` (no credentials), checks its SHA-256 and caches it by hash before running it in a sandboxed iframe (frontend repository, `docs/decisions/architecture/ADR-0005-game-host-bridge-protocol-v1.md`, accepted 2026-10-20). A cross-origin `fetch` needs the bucket to answer CORS.
- Local development works: fake-gcs answers `Access-Control-Allow-Origin: *` (checked 2026-10-20). No CORS configuration exists for a real bucket (no infrastructure code in this repository sets it).

**Risk / opportunity**
- In any real deployment, browsers refuse the download and no game ever loads, although every backend check passes.

**Recommendation**
- When the production (or any shared) bucket is provisioned, set a CORS rule on it: method `GET`, origin(s) = the web frontend's origin(s), response header `Content-Type`, a reasonable max age. Keep it limited to the frontend origins, not `*`.
- Verify with a browser (or `curl -H "Origin: <web origin>"`) against a signed URL before the first deployment.

**Why this horizon**
- Nothing is deployed yet; it must be done before the first deployment the web frontend uses, and it is easy to forget because local development hides it.

**Reevaluate when**
- The bucket is provisioned or the web frontend is first deployed against a non-emulated bucket.

**Next process**
- None (infrastructure configuration when deploying).

**Last reviewed**
2026-10-20

## LATER

The concern is legitimate, but current conditions do not justify acting on it.

A LATER item should have a meaningful reevaluation trigger where possible.

### Acting on reported game faults: verification and review

**Area**
Security / Game quality

**Current state / evidence**
- Players' clients report game frontend faults (unreachable, hash mismatch, no READY, bridge version, container violation, bridge abuse) on their live connection; they are stored in `play_game_faults`, one row per Session, reporter and kind (`play/docs/DATA_MODEL.md`, `play/docs/FLOWS.md` Game Fault Reports; 2026-10-20).
- A report is the client's claim. Nothing reads the table automatically; no game is flagged and no Session ends because of it, by the human's decision (a false report must cost nothing to the accused game).

**Risk / opportunity**
- Problems are auditable but only found when someone runs the audit queries; a bad game could keep being played meanwhile.
- A client could fabricate reports; raw counts alone do not tell a bad game from coordinated false reports.

**Recommendation**
- When games are published for real, add a review step fed by the table: flag a version for review once enough distinct reporters across distinct Sessions report it, never on one report.
- Confirm a flagged version objectively: its frontend package is an immutable HTML file, so a harness can run it headless in the same sandbox and bridge and check whether it really floods the bridge, opens frames or never answers READY. A false accusation then costs one verification run.
- Look at reporters too: one user reporting many versions points at the client.

**Why this horizon**
- No Game is published yet (Game Management is not built), so there is nothing to protect players from at scale; the evidence is being collected from now on.

**Reevaluate when**
- Game publishing exists, or the audit queries show a version with reports from several distinct reporters.

**Next process**
- PRODUCT_DISCUSSION (what happens to a flagged game) then DOMAIN_DESIGN (which domain owns game health).

**Last reviewed**
2026-10-20

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
