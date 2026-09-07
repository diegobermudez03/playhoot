# Feature Development

Use this when you want to take a concrete Playhoot capability from idea to approved implementation work, then through implementation and review.

Surface flow:

```text
CONVERSATIONAL AI
-> HUMAN design decisions
-> CODEBASE AGENT persistence as needed
-> CONVERSATIONAL AI/HUMAN READY checkpoint
-> CODEBASE AGENT implementation
-> INDEPENDENT REVIEWER
-> CODEBASE AGENT fixes if required
-> INDEPENDENT REVIEWER re-review
-> CODEBASE AGENT closure
```

If implementation or review discovers a DECISION_REQUIRED issue, it returns to CONVERSATIONAL AI + HUMAN before implementation continues.

Normally the Conversational Orchestrator selects and enters this process automatically from a natural request, usually as the graduation step of a broader initiative (see `docs/ai/README.md` and `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md`). Use this guide directly only for manual/advanced entry, debugging, or when you already know exactly which process you want.

## When to use it

- A concrete feature, migration, refactor, or technical capability needs design and implementation.
- Product, architecture, domain, and standard blockers are resolved or can be resolved before implementation.
- You want an implementation-ready WORK specification.

## Step 1 - HUMAN

Start in a CONVERSATIONAL AI. Paste:

```text
Repository:
https://github.com/diegobermudez03/playhoot

Repository ref:
<branch or commit>

Repository bootstrap:
Open the repository at the exact ref above and read `/AGENTS.md` first. Follow
its routing instructions to load only the context relevant to this task. Do
not scan repository documentation indiscriminately. If you cannot access the
repository/ref or a required file, tell me before continuing instead of
reasoning as if you had read it.

Process:
Playhoot Feature Development

Feature or technical capability:
[describe it]

Why it matters:
[user/business/technical reason]

Known constraints, accepted decisions, or out-of-scope items:
[optional]
```

## Step 2 - CONVERSATIONAL AI

The AI will challenge scope and placement first. If the feature is not ready, it will recommend the right product, architecture, domain, or standard process before implementation.

For non-trivial work, the Conversational AI prepares `docs/ai/workspaces/active/<feature-topic>/HUMAN_REVIEW.md` content as the human checkpoint surface. Expect:

- a Feature Design Review for scope, behavior, tradeoffs, and material decisions;
- a DRAFT WORK specification under `docs/work/active/` once the work is concrete enough;
- a READY Review that summarizes what you are approving without requiring you to inspect machine-oriented WORK internals line by line.

Persisting the workspace and creating/updating the DRAFT WORK file are CODEBASE AGENT steps; the Conversational AI gives you a CODEBASE AGENT HANDOFF for them.

## Step 3 - HUMAN

At the design checkpoint, approve, reject, ask questions, request modifications, defer, or route unresolved material questions to another process.

At the READY checkpoint, say explicitly whether the WORK is READY for implementation. READY is a HUMAN authorization — a DRAFT WORK specification is not implementation authority, and a Codebase Agent must not self-approve it.

## Step 4 - CODEBASE AGENT

After you approve READY, a CODEBASE AGENT marks the WORK READY. The workspace stays active through implementation and independent review — it is not removed merely because the WORK reached READY, implementation began, or review is pending. It is removed only once the WORK reaches DONE/CANCELLED and the surrounding initiative has no remaining tracked work (see `docs/ai/workspaces/README.md`).

Implementation then goes to a CODEBASE AGENT operating on the checkout, using the minimal handoff from `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`:

```text
Implement the approved Playhoot work specification:

docs/work/active/WORK-NNNN-short-title.md

It is READY.

Follow:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md
```

## Step 5 - INDEPENDENT REVIEWER

Independent review, normally performed by a fresh Codebase Agent operating read-only, follows the implementation/review protocol.

- A REQUIRED_FIX finding returns to CODEBASE AGENT for a fix and re-review.
- A DECISION_REQUIRED finding returns to CONVERSATIONAL AI + HUMAN before affected implementation continues.
- An APPROVED verdict allows CODEBASE AGENT closure.

DONE requires an APPROVED independent review and no unresolved REQUIRED_FIX or DECISION_REQUIRED findings. NON_BLOCKING suggestions are optional.

## Resume in a new session - CONVERSATIONAL AI

Paste into a CONVERSATIONAL AI:

```text
Repository:
https://github.com/diegobermudez03/playhoot

Repository ref:
<branch or commit>

Repository bootstrap:
Open the repository at the exact ref above and read `/AGENTS.md` first. Follow
its routing instructions before resuming. If you cannot access the
repository/ref or a required file, tell me instead of assuming its contents.

Resume the Playhoot Feature Development process from:
docs/ai/workspaces/active/<process-topic>/
```
