# Architecture Discussion

Use this when you want to decide how Playhoot should be structured technically.

Surface flow: CONVERSATIONAL AI <-> HUMAN, with CODEBASE AGENT for repository persistence/synchronization.

## When to use it

- A question affects system structure, ownership, coupling, persistence, infrastructure, or public contracts.
- You want help comparing architecture alternatives before implementation.
- The issue is broader than a local code detail.

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
Playhoot Architecture Discussion

Architecture question:
[describe it]

Trigger or concern:
[why this came up]

Options I am considering:
[optional]
```

## Step 2 - CONVERSATIONAL AI

For small questions, the AI may answer directly with a recommendation.

For material decisions, the Conversational AI prepares the checkpoint content — the underlying problem, options, tradeoffs, recommendation, affected boundaries, and any decision needed from you — for `docs/ai/workspaces/active/<architecture-topic>/HUMAN_REVIEW.md`. It gives you a CODEBASE AGENT HANDOFF to persist it.

## Step 3 - HUMAN

Review the checkpoint and respond with one of:

- approve an option;
- reject the recommendation;
- defer the decision;
- ask questions;
- request modifications;
- route to domain design, technical exploration, standard-setting, or feature development.

## Step 4 - CONVERSATIONAL AI / CODEBASE AGENT

If you approve a material architecture decision, the AI will route any decision record, canonical architecture/domain documentation, or follow-up implementation work to the appropriate owner. Repository-side persistence of that outcome is a CODEBASE AGENT step, normally reached through a CODEBASE AGENT HANDOFF from the Conversational AI.

Approval of architecture does not mean the implementation already exists.

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

Resume the Playhoot Architecture Discussion process from:
docs/ai/workspaces/active/<process-topic>/
```
