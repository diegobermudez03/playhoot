# Conversational Orchestrator Protocol

Status: AGENT EXECUTION PROTOCOL

This is an internal protocol, not a human process. A fresh Conversational AI
receiving a normal Playhoot request uses this protocol before asking the
human to choose a process, unless it is clearly resuming a known persisted
initiative.

This is how the human normally experiences Playhoot: talk naturally, and the
Conversational AI determines which internal process applies, moves between
processes when needed, and tells the human when a Codebase Agent action is
required. See `docs/ai/README.md` for the human-facing entry point this
protocol serves.

## Not A Keyword Classifier

Route from the human's GOAL, current repository state, and existing open
initiatives/WORK — not from matching nouns to process names.

## Ownership

The Conversational Orchestrator owns:

- initial request routing;
- detecting an existing related open initiative or WORK before starting new
  work;
- resume behavior;
- selecting the internal Playhoot process that currently owns the concern;
- transitioning between processes/subprocesses and returning to the parent
  process;
- downstream consequence analysis after a material milestone;
- initiative-level implementation planning (decomposition into a Project's
  ordered WORK, per `docs/projects/README.md`);
- selecting the next appropriate action/process;
- open-initiative discovery;
- determining when an initiative is actually resolved.

The human normally interacts with this protocol simply by talking. Internal
process names are not required for normal use.

## The Durable Unit: Initiative

The durable conversational/work unit is an INITIATIVE, not a single internal
process. Examples: `session-runtime-v1`, `observability-hardening`,
`ai-workflow-simplification`, `game-creation`.

An initiative may internally move through Architecture Discussion, Domain
Design, Architecture Discussion again, implementation planning, Feature
Development, implementation/review, and another Feature Development pass on
the next WORK — without creating a new human-facing workflow every time.

Prefer ONE active workspace for one coherent initiative:

`docs/ai/workspaces/active/<initiative>/`

Create a separate child workspace only when a sub-concern becomes
independently resumable/concurrent work in its own right. See
`docs/ai/workspaces/README.md` for workspace file semantics, the
`AI_CONTEXT.md` resume header, and the persistence threshold — this protocol
does not redefine those.

## Routing Responsibilities

1. Determine whether the request is a continuation of an existing open
   initiative, interaction with existing WORK, or genuinely new work.
2. Search `docs/ai/workspaces/active/`, `docs/projects/active/` (each
   Project's `PROJECT.md` and `works/`), and `docs/work/active/` (standalone
   WORK) for matching state before creating duplicate work.
3. Select the internal process that currently owns the concern.
4. Load the matching human-facing process guide (`docs/ai/processes/*`) and
   agent protocol (`docs/ai/protocols/*`).
5. Transition to another process/subprocess when a different concern becomes
   the correct owner, and return to the parent process/initiative once the
   sub-concern resolves.
6. Never require the human to understand process taxonomy merely to use the
   system.
7. Surface material human decisions naturally, in plain language.
8. Preserve execution-surface boundaries (`docs/ai/OPERATING_MODEL.md`) — the
   Orchestrator itself never mutates the repository; it produces a Codebase
   Agent Handoff.
9. Never bypass accepted workflow governance merely because routing is
   automatic now.

## Routing Intent Guidance

Guidance, not a keyword table. A request may match more than one; use
judgment, not string matching:

- PRODUCT_DISCUSSION — product behavior, user value, market/scope, feature
  meaning.
- ARCHITECTURE_DISCUSSION — system-level technical direction,
  component/layer responsibility, cross-cutting technical structure, design
  of a larger technical initiative.
- DOMAIN_DESIGN — business concepts, bounded contexts, ownership, domain
  language, invariants.
- GUIDED_TECHNICAL_EXPLORATION — unfamiliar technical area,
  technology/pattern exploration, human wants the AI to teach/map options
  before deciding.
- FEATURE_DEVELOPMENT — a sufficiently concrete capability that may become
  implementable WORK.
- ENGINEERING_STANDARD — reusable engineering/design/coding rule intended to
  govern future work.
- PRINCIPAL_ENGINEER_REVIEW — broad proactive technical assessment, "what
  are we missing?", reliability/observability/security/infrastructure/quality
  review where the AI should identify concerns rather than only respond to a
  predetermined solution.
- AI_WORKFLOW_CHANGE — changes to this workflow/AI operating system itself.

A single request may move through several of these before it is resolved.

## Process Transitions

Typical transitions include:

- ARCHITECTURE_DISCUSSION -> DOMAIN_DESIGN -> back to
  ARCHITECTURE_DISCUSSION
- ARCHITECTURE_DISCUSSION -> GUIDED_TECHNICAL_EXPLORATION -> back to
  ARCHITECTURE_DISCUSSION
- PRINCIPAL_ENGINEER_REVIEW -> GUIDED_TECHNICAL_EXPLORATION ->
  ENGINEERING_STANDARD or FEATURE_DEVELOPMENT
- FEATURE_DEVELOPMENT -> a material prerequisite process -> back to
  FEATURE_DEVELOPMENT

Do not expose these transitions mechanically to the human unless useful.
Explain WHY the conversation needs to address something before continuing,
not which internal process name was entered.

When routing temporarily enters another process, persist enough state in
`AI_CONTEXT.md` to know: current process, parent process, why the transition
happened, what condition allows return, and where to return. Prefer updating
the single initiative workspace rather than creating a new one for every
small internal transition.

## Proactive Principal Engineer Behavior (Preserved)

Routing must preserve the Principal Engineer Contract in
`docs/ai/OPERATING_MODEL.md`. The Conversational AI still challenges the
user's framing, looks one level above, proposes alternatives the human did
not mention, teaches relevant concepts, distinguishes actual need from
sophistication, and makes concrete recommendations. Do not turn the workflow
into "user asks X -> mechanically execute X." If the proposed framing is
wrong or incomplete, challenge it. Human material authority is unchanged.

## Downstream Consequence Check

A material milestone (an accepted product/architecture/domain decision or
engineering standard) is not initiative termination.

After every material milestone, determine:

- what durable knowledge must be synchronized;
- whether current code now differs from the accepted direction;
- whether implementation consequences exist;
- whether additional design/exploration is required;
- whether the result should be decomposed into implementation work;
- the best next action.

Explain the meaningful next step naturally. If the human says something like
"continue", "proceed", "what's next?", "let's implement it", or "turn this
into tasks", transition accordingly without requiring the human to name an
internal process.

## Initiative Implementation Planning: Graduating Into A Project

For an accepted larger direction, transform it into coherent, ordered WORK
using accepted decisions/canonical knowledge, actual current implementation,
current-state docs, dependencies, risk, and useful vertical boundaries. Do
not mechanically decompose by endpoint, file, package, or repository method
unless that genuinely is the right boundary. Prefer coherent
capabilities/outcomes.

### Project Replaces PLAN.md

Once an initiative needs multiple related, ordered implementation WORK
items, it graduates into a Project (`docs/projects/README.md`):

`docs/projects/active/<project-slug>/PROJECT.md` plus `works/`.

`PROJECT.md` is the durable, human-facing dashboard for the initiative's
implementation roadmap - it replaces the earlier ad hoc practice of tracking
a numbered "Slice" sequence in a workspace `PLAN.md`. `PROJECT.md` owns
initiative implementation goal, the ordered/grouped WORK table, dependencies,
sequencing rationale, intentionally-excluded scope, and the project
completion criterion. It does not own accepted architecture/domain/product
truth, a WORK's detailed implementation contract, or a WORK's own
status/source-level progress - those remain owned by their existing sources
(ADRs/canonical docs, the WORK file itself, Git).

An initiative still exploratory - architecture/domain design not yet decided
to require concrete implementation WORK - remains a
`docs/ai/workspaces/active/<initiative>/` process; it graduates into a
Project only once it needs an ordered set of required implementation WORK
(see `docs/projects/README.md`).

### Known Future Outcomes Get A PLANNED WORK Immediately

Do not defer materializing a WORK merely because its design has not started.
When a required implementation outcome becomes known - discovered during
design, during implementation of another WORK, or during an audit of an
active Project's remaining scope - create it as a `PLANNED` WORK immediately
under the Project (`docs/projects/README.md` Invariant 1) and place/order it
in `PROJECT.md`, then continue whatever else is in progress if appropriate.
Do not let a known required outcome exist only as prose in `PROJECT.md`, an
`AI_CONTEXT.md`, an ADR, or a handoff.

### Design/Refine Just In Time

Default to planning the Project's roadmap broadly (as PLANNED WORK) but
designing each WORK in detail (PLANNED -> DRAFT) just in time. For example,
given a Project with WORK-A through WORK-D all PLANNED, it may be appropriate
to move only WORK-A to DRAFT now. After WORK-A's implementation/review,
inspect what was learned, reevaluate the remaining PLANNED WORK if necessary,
then design/refine the next one.

Do not freeze detailed DRAFT specifications for every future WORK when later
implementation evidence may change them. Moving multiple WORK items to DRAFT
up front is allowed when their contracts are already genuinely stable and
doing so adds value, but it is not the default.

## Feature Development As Internal Graduation

Feature Development is the internal governed mechanism used to move a
PLANNED WORK through design (DRAFT) to concrete implementable WORK (READY).
The human does not normally say "start Feature Development." Instead:

Human: "Let's design/implement the next WORK."

Orchestrator: identifies the next PLANNED WORK from the active Project's
`PROJECT.md` (or accepts a genuinely new WORK outside any Project), loads
Feature Development internally, moves it PLANNED -> DRAFT, determines
whether material design is already resolved, asks only unresolved material
questions, persists the WORK through a Codebase Agent Handoff, drives it to
the READY checkpoint, and after human READY, routes to implementation.

All existing WORK/READY governance in `docs/work/README.md`,
`docs/ai/processes/FEATURE_DEVELOPMENT.md`, and
`docs/ai/protocols/IMPLEMENTATION_REVIEW.md` is unchanged.

## Initiative Loop After WORK Completion

When a WORK reaches DONE, do not automatically conclude the initiative or
Project. Return control to `PROJECT.md`. Determine:

- did the WORK satisfy the entire Project goal?
- did implementation produce learning that changes remaining PLANNED WORK?
- did implementation or review surface a newly-known-required outcome that
  needs its own new PLANNED WORK (Invariant 1)?
- what PLANNED/DRAFT WORK remains?
- is another design/exploration step required?
- is the Project now complete (see its completion criterion)?

If more work remains, update `PROJECT.md`'s WORK table/status and initiative
continuity (`internal/AI_CONTEXT.md` if present) and guide the human toward
the next meaningful step.

Only move the Project to `docs/projects/completed/` when its completion
criterion is satisfied or the Project is cancelled, no known downstream work
intentionally remains tracked, and required durable knowledge has been
promoted to its owners. See `docs/projects/README.md` and the Durable Active
Process Invariant / Workspace Lifetime rules in
`docs/ai/workspaces/README.md` for the equivalent rule while an initiative is
still a pre-Project workspace.

## Open Initiative/Process Discovery

Support natural requests such as: "What processes/initiatives are open?",
"What is ready for planning?", "What is ready to implement?", "What is
currently being implemented?", "What is blocked?", "What tasks remain for
X?", "Where did we stop?", "Resume the session work."

Primary sources: `docs/ai/workspaces/active/` for pre-Project exploratory
initiatives, and `docs/projects/active/<project-slug>/PROJECT.md` for any
initiative that has graduated into a Project. For each active workspace,
read `AI_CONTEXT.md`'s resume header (see `docs/ai/workspaces/README.md`)
and summarize topic, current internal process/stage, related WORK/decision/
artifacts, blocker/current checkpoint, next actor, and next action. For each
active Project, read `PROJECT.md`'s Current Work and WORK table directly,
and its `internal/AI_CONTEXT.md` resume header if present.

Cross-check `docs/work/active/`/`docs/work/completed/` (standalone WORK) and
each Project's `works/` directory for actual WORK lifecycle. If an active
WORK exists without an active workspace/Project under the Durable Active
Process Invariant, report:

```text
PROCESS CONTINUITY DRIFT
```

and reconstruct a minimal workspace only when the durable artifacts make the
state unambiguous. Do not invent missing conversational history.

An active WORK is implementation authority/state. An active workspace is
process continuity. Neither replaces the other. Do not create a
duplicated, manually-maintained active-initiative registry — discover state
from these owning directories.

## Natural Continuation

The human may respond naturally: "continue", "proceed", "what's next?",
"let's plan the implementation", "let's start implementing", "what tasks are
left?", "where were we?", "resume this". Interpret these from persisted
initiative context. Do not require command syntax or internal process names.

## Non-Goals

- Do not collapse the existing Playhoot processes into one giant protocol.
- Do not remove governance, weaken material decision gates, weaken READY, or
  weaken independent review.
- Do not invent WORK statuses beyond the set `docs/work/README.md` defines
  (PLANNED, DRAFT, READY, IMPLEMENTING, DONE, CANCELLED); that set changes
  only through the AI_WORKFLOW_CHANGE process, not ad hoc during routing.
- Do not create a manually maintained active-process registry.
- Do not duplicate exact Git diffs into `AI_CONTEXT.md`.
- Do not require every trivial conversation to be persisted.
- Do not implement product features as a side effect of routing.
