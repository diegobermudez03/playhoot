# Product Discussion

Use this when you want to decide what Playhoot should do.

Surface flow: CONVERSATIONAL AI <-> HUMAN, with CODEBASE AGENT only when repository persistence/synchronization is needed.

Normally the Conversational Orchestrator selects and enters this process automatically from a natural request (see `docs/ai/README.md`). Use this guide directly only for manual/advanced entry, debugging, or when you already know exactly which process you want.

## When to use it

- You are considering product scope, user value, creator/player experience, launch target, or priority.
- You want to accept, reject, defer, or reshape a product idea before implementation.
- The main question is "should Playhoot do this?", not "how should the system be structured?"

## Step 1 - HUMAN

Paste into a CONVERSATIONAL AI:

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
Playhoot Product Discussion

Product question or idea:
[describe it]

Why it matters:
[user/business reason]

Known constraints or timing:
[optional]
```

## Step 2 - CONVERSATIONAL AI

For simple questions, the AI may answer directly with a recommendation.

For non-trivial decisions, the Conversational AI prepares the checkpoint content — the product problem, options, tradeoffs, recommendation, and any material decision needed from you — for `docs/ai/workspaces/active/<product-topic>/HUMAN_REVIEW.md`. It gives you a CODEBASE AGENT HANDOFF to persist it.

## Step 3 - HUMAN

Review the recommendation and respond with one of:

- approve the product direction;
- reject it;
- defer it;
- ask questions;
- request modifications;
- route it to another process.

## Step 4 - CONVERSATIONAL AI / CODEBASE AGENT

If you approve a material product decision, the AI will route any durable decision record, canonical product documentation, or follow-up implementation work to the appropriate owner. Repository-side persistence of that outcome is a CODEBASE AGENT step, normally reached through a CODEBASE AGENT HANDOFF from the Conversational AI.

Approval of product direction does not automatically implement anything.

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

Resume the Playhoot Product Discussion process from:
docs/ai/workspaces/active/<process-topic>/
```
