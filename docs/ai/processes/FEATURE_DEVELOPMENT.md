# Feature Development

Use this when you want to take a concrete Playhoot capability from idea to approved implementation work.

## When to use it

- A concrete feature, migration, refactor, or technical capability needs design and implementation.
- Product, architecture, domain, and standard blockers are resolved or can be resolved before implementation.
- You want an implementation-ready WORK specification.

## Step 1 - Start

Paste:

```text
I want to use the Playhoot Feature Development process.

Feature or technical capability:
[describe it]

Why it matters:
[user/business/technical reason]

Known constraints, accepted decisions, or out-of-scope items:
[optional]
```

## Step 2 - What the AI will give you

The AI will challenge scope and placement first. If the feature is not ready, it will recommend the right product, architecture, domain, or standard process before implementation.

For non-trivial work, the AI will maintain `docs/ai/workspaces/active/<feature-topic>/HUMAN_REVIEW.md` as the human checkpoint surface. Expect:

- a Feature Design Review for scope, behavior, tradeoffs, and material decisions;
- a DRAFT WORK specification under `docs/work/active/` once the work is concrete enough;
- a READY Review that summarizes what you are approving without requiring you to inspect machine-oriented WORK internals line by line.

## Step 3 - What you need to do

At the design checkpoint, approve, reject, ask questions, request modifications, defer, or route unresolved material questions to another process.

At the READY checkpoint, say explicitly whether the WORK is READY for implementation.

A DRAFT WORK specification is not implementation authority.

## Step 4 - What happens next

After you approve READY, the AI marks the WORK READY. The temporary workspace may be removed once the WORK and relevant canonical context are sufficient durable handoff.

Implementation can then start with:

```text
Implement the approved Playhoot work specification:

docs/work/active/WORK-NNNN-short-title.md

It is READY.

Follow:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md
```

Implementation, independent review, required fixes, and closure follow the implementation/review protocol. DONE requires an APPROVED independent review and no unresolved REQUIRED_FIX or DECISION_REQUIRED findings. NON_BLOCKING suggestions are optional.

## Resume in a new session

```text
Resume the Playhoot Feature Development process from:
docs/ai/workspaces/active/<process-topic>/
```
