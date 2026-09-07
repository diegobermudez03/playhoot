# AI Process Workspaces

Status: TEMPORARY WORKFLOW CONTEXT (DURABLE WHILE ACTIVE)

AI process workspaces live under:

`docs/ai/workspaces/active/<initiative>/`

They are temporary, non-canonical, and non-authoritative — but once a process
crosses the persistence threshold below, the workspace is the durable
continuity record for that process until it resolves. It is not deleted
merely because one internal stage finished.

One workspace normally covers one coherent INITIATIVE, not a single internal
process. An initiative may move through several internal processes
(Architecture Discussion, Domain Design, Guided Technical Exploration,
Feature Development, implementation, review, and back) without a new
workspace for every transition. See
`docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md` for how routing,
transitions, and initiative planning work. Create a separate child workspace
only when a sub-concern becomes independently resumable/concurrent work in
its own right.

## Durable Active Process Invariant

A process may remain chat-only while it is genuinely exploratory, trivial,
single-session, and has produced no repository effect or durable material
checkpoint.

BEFORE a process performs substantive repository mutation, delegates
repository-side execution, or otherwise requires durable cross-session
continuity, it must have persisted active-process state under this
directory. From that point until the process is terminal/resolved, it
remains discoverable and resumable from the repository. The repository is
the durable process memory.

Creating/refreshing the workspace may itself be the first repository-side
action of a Codebase Agent Handoff. A handoff for repository-changing work
should identify the active workspace, or instruct the Codebase Agent to
create it before substantive changes.

This applies to workflow persistence, canonical documentation changes,
decision persistence, WORK creation/update, implementation, tests/refactors,
current-state documentation changes, Radar mutation, and other substantive
repository changes. Do not require the human to create these files manually.

## Default Files

- `AI_CONTEXT.md` — mandatory for every persisted active process.
- `HUMAN_REVIEW.md` — required only while a human-facing checkpoint/decision/
  review is currently pending. It does not need to exist merely to satisfy a
  file-pairing rule during stages where the human has no open checkpoint
  (for example, mid-implementation awaiting independent review).
- `PLAN.md` — optional; used only for an initiative with multiple
  implementation slices. See
  `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md` for what it owns and how
  it is used. It is temporary, non-canonical, non-authoritative, and must not
  duplicate WORK content or accepted decisions.

Do not create `docs/ai/workspaces/completed/`. Do not create a second,
competing process-state system alongside this one.

## HUMAN_REVIEW.md

`HUMAN_REVIEW.md` is for the human.

It should contain the current checkpoint or proposal in a form the human can
understand and decide on. Include only what is useful for the checkpoint,
such as:

- process/topic;
- checkpoint;
- problem or desired outcome;
- recommendation or proposal;
- Mermaid diagrams when they help;
- contracts, API shape, schema, or examples when material to the decision;
- alternatives and tradeoffs;
- material decisions requiring approval;
- out-of-scope items;
- what happens if approved;
- next human action.

Do not fill it with generic AI mechanics, internal loading instructions, or
template-authoring guidance.

## AI_CONTEXT.md

`AI_CONTEXT.md` is for future AI sessions.

### Resume Header

Start every active `AI_CONTEXT.md` with a concise resume header:

```text
Process:
Topic:
Current stage:
Current execution surface:
Parent process: <optional>
Return to: <optional>
Related durable artifacts:
Blocked by:
Next action:
Last durable checkpoint:
Last updated:
```

Directory presence means the process is active; these fields explain where
it currently stands. Do not build a complicated status enum beyond this —
keep values concise and semantic (for example, `Current stage: awaiting
independent review`). A fresh agent reading this header should never be left
asking "now what?"

After the header, keep the existing useful context:

- canonical references;
- evidence inspected;
- inherited constraints;
- human decisions;
- unresolved material questions;
- planning conclusions;
- likely touchpoints;
- verification notes;
- pending effects;
- resume context.

Do not put hidden material decisions in `AI_CONTEXT.md`. Do not include
private chain-of-thought — record conclusions, evidence, constraints, and
planning context only. Avoid duplicating canonical knowledge; reference
durable owners instead.

`AI_CONTEXT.md` is not implementation authority and does not weaken Codebase
Agent autonomy.

### Process Stack / Subprocesses

When routing temporarily enters another process, the header and surrounding
context should make clear: the current process, the parent process, why the
transition happened, what condition allows return, and where to return.
Prefer updating the single initiative workspace over creating a new one for
every small internal transition.

### Do Not Duplicate Git

`AI_CONTEXT.md` records PROCESS STATE, not a hand-written diff log: what was
attempted, the meaningful milestone completed, current phase, material/local
discoveries worth carrying forward, verification state, blocker, pending
action, and next actor. Git/worktree/diff remains the source for exact
file-level implementation changes. Do not record every edited function/file
merely because it changed.

## Surface Ownership

A Conversational AI owns/prepares the semantic content of
`HUMAN_REVIEW.md`/`AI_CONTEXT.md`/`PLAN.md` — the design/conversation
process, checkpoint, and resumable context.

Persisting, updating, or removing these workspace files is always a Codebase
Agent action. The Conversational AI produces a Codebase Agent Handoff (see
`docs/ai/OPERATING_MODEL.md`) asking a Codebase Agent to persist or update
them. Performing that persistence step does not give the Codebase Agent
authority over the material decision being recorded — it remains whatever
the Conversational AI/human already decided.

## Cross-Session Rule

Before asking the human for a material decision in a non-trivial process,
persist enough context in the workspace for a fresh AI agent to resume.

When the human makes a material decision, persist the semantic result before
relying on future chat memory.

If the decision changes accepted product, architecture, domain,
engineering-standard, workflow, or implementation authority, synchronize the
durable owner required by that process. The workspace is not enough.

## Workspace Lifetime

The same active initiative workspace normally survives the whole active
process, including a Feature Development slice's full lifecycle:

design -> DRAFT -> human READY -> READY -> IMPLEMENTING -> independent review
-> fixes/re-review -> DONE/CANCELLED

Do not remove it merely because:

- a WORK reaches READY;
- implementation begins;
- implementation is awaiting independent review.

The workspace references the WORK rather than duplicating its approved
contract.

## Remote Access Limit

A fresh Codebase Agent on the same checkout may inspect uncommitted worktree
state. A remote Conversational AI can only see process/code state available
through the supplied accessible repository ref or other provided evidence. An
active workspace that exists only locally and was never made accessible
cannot be read remotely. Distinguish "persisted in checkout" from "remotely
accessible at supplied ref" without inventing a Git commit/push policy. When
remote continuation is required, guide the human to make the relevant state
accessible.

## Promotion And Cleanup

Workspaces are not permanent archives.

Remove the temporary workspace only after the overall initiative has reached
a durable terminal/resolved state — the initiative goal is satisfied or
cancelled, no known downstream work intentionally remains tracked, and all
required information has been promoted to canonical or durable owners.

For a Feature Development slice specifically: when its WORK reaches
DONE/CANCELLED, move it to `docs/work/completed/` and synchronize
canonical/current-state docs as required, per usual rules — then return to
the initiative plan (see the Orchestrator's initiative loop in
`docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md`) rather than assuming the
workspace can be removed immediately. Remove the workspace only once the
initiative itself is resolved by the rule above.

Examples of durable owners include accepted product/architecture/domain/
standard documentation, decision records, WORK specifications, current-state
documentation after implementation, and the Engineering Radar where
appropriate.

Historical memory belongs in those owners and Git history, not in completed
workspace copies.
