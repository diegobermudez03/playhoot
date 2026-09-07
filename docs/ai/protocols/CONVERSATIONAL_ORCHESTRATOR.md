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
- initiative-level implementation planning (decomposition into slices);
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
Development, implementation/review, and another Feature Development slice —
without creating a new human-facing workflow every time.

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
2. Search `docs/ai/workspaces/active/` (and `docs/work/active/` when
   implementation state matters) for matching state before creating
   duplicate work.
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

## Initiative Implementation Planning

For an accepted larger direction, transform it into coherent, incremental
implementation slices using accepted decisions/canonical knowledge, actual
current implementation, current-state docs, dependencies, risk, and useful
vertical boundaries. Do not mechanically decompose by endpoint, file,
package, or repository method unless that genuinely is the right boundary.
Prefer coherent capabilities/outcomes.

### Optional PLAN.md

For an initiative requiring multiple implementation slices, allow:

`docs/ai/workspaces/active/<initiative>/PLAN.md`

PLAN.md is TEMPORARY, NON-CANONICAL, NON-AUTHORITATIVE. It owns only
initiative-level implementation decomposition:

- initiative implementation goal;
- ordered/coherent slices;
- important dependencies;
- sequencing rationale;
- intentionally deferred/out-of-scope slices;
- links to WORK specs once they exist.

It does not own accepted architecture/domain/product truth, detailed
implementation contracts, actual WORK status, source-level progress, or exact
diffs. Actual WORK lifecycle/status remains owned by `docs/work/`. Do not
duplicate WORK content into PLAN.md.

### Just-In-Time WORK Specs

Default to planning the initiative broadly but materializing detailed WORK
specifications just in time. For example, given a plan with slices S1-S4, it
may be appropriate to create detailed WORK only for S1. After S1's
implementation/review, inspect what was learned, reevaluate the downstream
plan if necessary, then materialize/refine the next slice.

Do not freeze detailed WORK specifications for every future slice when later
implementation evidence may change them. Creating multiple WORK specs up
front is allowed when their contracts are already genuinely stable and doing
so adds value, but it is not the default.

## Feature Development As Internal Graduation

Feature Development is the internal governed mechanism used when a planned
slice is ready to become concrete implementable WORK. The human does not
normally say "start Feature Development." Instead:

Human: "Let's implement the next slice."

Orchestrator: identifies the next slice, loads Feature Development
internally, determines whether material design is already resolved, asks
only unresolved material questions, produces/persists the WORK through a
Codebase Agent Handoff, drives it to the READY checkpoint, and after human
READY, routes to implementation.

All existing WORK/READY governance in `docs/work/README.md`,
`docs/ai/processes/FEATURE_DEVELOPMENT.md`, and
`docs/ai/protocols/IMPLEMENTATION_REVIEW.md` is unchanged.

## Initiative Loop After WORK Completion

When a WORK reaches DONE, do not automatically conclude the initiative.
Return control to the initiative plan. Determine:

- did the WORK satisfy the entire initiative goal?
- did implementation produce learning that changes remaining slices?
- what planned slice remains?
- is another design/exploration step required?
- is the initiative now complete?

If more work remains, update initiative continuity (`AI_CONTEXT.md`, and
`PLAN.md` if present) and guide the human toward the next meaningful step.

Only remove the active initiative workspace when the initiative goal is
satisfied or cancelled, no known downstream work intentionally remains
tracked, and required durable knowledge has been promoted to its owners. See
the Durable Active Process Invariant and Workspace Lifetime rules in
`docs/ai/workspaces/README.md`.

## Open Initiative/Process Discovery

Support natural requests such as: "What processes/initiatives are open?",
"What is ready for planning?", "What is ready to implement?", "What is
currently being implemented?", "What is blocked?", "What tasks remain for
X?", "Where did we stop?", "Resume the session work."

Primary source: `docs/ai/workspaces/active/`. For each active workspace, read
`AI_CONTEXT.md`'s resume header (see `docs/ai/workspaces/README.md`) and
summarize topic, current internal process/stage, related WORK/decision/
artifacts, blocker/current checkpoint, next actor, and next action. When
`PLAN.md` exists, use it for initiative decomposition.

Cross-check `docs/work/active/` and `docs/work/completed/` for actual WORK
lifecycle. If an active WORK exists without an active workspace under the
Durable Active Process Invariant, report:

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
- Do not invent new WORK statuses.
- Do not create a manually maintained active-process registry.
- Do not duplicate exact Git diffs into `AI_CONTEXT.md`.
- Do not require every trivial conversation to be persisted.
- Do not implement product features as a side effect of routing.
