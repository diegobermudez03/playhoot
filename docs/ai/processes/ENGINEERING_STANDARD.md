# Engineering Standard

Use this when you want to create or change a reusable Playhoot engineering rule.

## When to use it

- A repeatable coding, testing, documentation, error-handling, or design rule is needed across future work.
- Existing practice is inconsistent and the inconsistency matters.
- You want to decide enforcement and migration strategy separately from implementation.

## Step 1 - Start

Paste:

```text
I want to use the Playhoot Engineering Standard process.

Problem or inconsistency:
[describe it]

Examples or affected areas:
[optional]

Should this affect future code only or existing code too?
[optional]
```

## Step 2 - What the AI will give you

For small clarifications, the AI may answer directly.

For material standards, the AI will prepare `docs/ai/workspaces/active/<standard-topic>/HUMAN_REVIEW.md` with the problem, current pattern, options, recommended rule, enforcement options, and migration strategy.

## Step 3 - What you need to do

Review the recommendation and respond with one of:

- accept the standard;
- reject it;
- request wording changes;
- choose or change enforcement;
- choose future-only, opportunistic, or mandatory migration;
- route implementation work to Feature Development.

## Step 4 - What happens next

Accepted reusable standards are synchronized to `docs/engineering/standards/`.

Accepting a standard does not automatically authorize repository-wide migration or refactoring.

## Resume in a new session

```text
Resume the Playhoot Engineering Standard process from:
docs/ai/workspaces/active/<process-topic>/
```
