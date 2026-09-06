# Engineering Standard

Use this when you want to create or change a reusable Playhoot engineering rule.

Surface flow: CONVERSATIONAL AI <-> HUMAN, then CODEBASE AGENT to persist an accepted standard or separately execute approved migration work.

## When to use it

- A repeatable coding, testing, documentation, error-handling, or design rule is needed across future work.
- Existing practice is inconsistent and the inconsistency matters.
- You want to decide enforcement and migration strategy separately from implementation.

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
Playhoot Engineering Standard

Problem or inconsistency:
[describe it]

Examples or affected areas:
[optional]

Should this affect future code only or existing code too?
[optional]
```

## Step 2 - CONVERSATIONAL AI

For small clarifications, the AI may answer directly.

The Conversational AI prepares the recommendation/checkpoint content — the problem, current pattern, options, recommended rule, enforcement options, and migration strategy — for `docs/ai/workspaces/active/<standard-topic>/HUMAN_REVIEW.md`. For a non-trivial persistent checkpoint, it gives you a CODEBASE AGENT HANDOFF to persist `HUMAN_REVIEW.md` and `AI_CONTEXT.md`.

## Step 3 - HUMAN

Review the recommendation and respond with one of:

- accept the standard;
- reject it;
- request wording changes;
- choose or change enforcement;
- choose future-only, opportunistic, or mandatory migration;
- route implementation work to Feature Development.

## Step 4 - CODEBASE AGENT

Accepted reusable standards are always persisted to `docs/engineering/standards/` by a CODEBASE AGENT, via a CODEBASE AGENT HANDOFF from the Conversational AI.

Accepting a standard does not automatically authorize repository-wide migration or refactoring; migration work goes through Feature Development and a separate CODEBASE AGENT implementation step.

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

Resume the Playhoot Engineering Standard process from:
docs/ai/workspaces/active/<process-topic>/
```
