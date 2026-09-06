# Guided Technical Exploration

Use this when you want the AI to lead you through an unfamiliar technical area.

Surface flow: CONVERSATIONAL AI <-> HUMAN, with CODEBASE AGENT when local repository evidence or persistent artifacts must be inspected/changed.

## When to use it

- You know the area matters but do not know the right questions yet.
- You want the AI to map concerns, explain tradeoffs, and recommend next steps.
- Examples include observability, caching, storage options, security, backups, tracing, or deployment infrastructure.

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
Playhoot Guided Technical Exploration

Technical area or concern:
[describe it]

What I already know:
[optional]

How deep the first pass should be:
[optional]
```

## Step 2 - CONVERSATIONAL AI

The AI will lead the exploration, explain relevant concepts, identify gaps and adequate areas, compare options, and classify recommendations.

For non-trivial exploration, the AI may prepare `docs/ai/workspaces/active/<exploration-topic>/HUMAN_REVIEW.md` with a clear recommendation and suggested next process. If local repository evidence must be inspected, or the workspace needs to be persisted and the Conversational AI cannot write to the repository, it will give you a CODEBASE AGENT HANDOFF.

## Step 3 - HUMAN

Choose what deserves follow-up:

- no action;
- add or update a non-authoritative radar recommendation;
- route to architecture discussion;
- route to domain design;
- route to engineering standard;
- route to feature development;
- ask for another exploration pass.

## Step 4 - CONVERSATIONAL AI

Exploration does not itself approve architecture, standards, product behavior, or implementation.

Actionable recommendations move into the relevant decision or implementation process only if you choose to continue.

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

Resume the Playhoot Guided Technical Exploration process from:
docs/ai/workspaces/active/<process-topic>/
```
