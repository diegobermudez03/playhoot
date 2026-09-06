# AI Workflow Change

Use this when you want to change Playhoot's AI-assisted development workflow.

## When to use it

- You want to change AI roles, authority, review gates, process routing, handoff prompts, workflow artifacts, templates, lifecycle semantics, or documentation synchronization behavior.
- You are fixing a material ambiguity in how humans, ChatGPT, Codex, or reviewers should work.
- The change is about the AI workflow itself, not product behavior or Playhoot system architecture.

## Step 1 - Start

Paste:

```text
I want to use the Playhoot AI Workflow Change process.

Requested workflow change:
[describe it]

Problem with the current workflow:
[describe it]

Desired outcome:
[describe it]

Files or prompts I think may be affected:
[optional]
```

## Step 2 - What the AI will give you

For editorial fixes, the AI may make the correction directly.

For material workflow changes, the AI will prepare `docs/ai/workspaces/active/<workflow-topic>/HUMAN_REVIEW.md` with the current workflow problem, impact map, minimal proposal, compatibility notes, validation plan, and decisions needed from you.

## Step 3 - What you need to do

Review the proposal and respond with one of:

- approve the workflow change;
- reject it;
- request modifications;
- defer it;
- ask questions.

Material workflow changes require explicit human approval before they are applied.

## Step 4 - What happens next

After approval, the AI updates the affected AI workflow artifacts coherently, validates references and routing, performs conceptual dry runs, and records the material workflow change in `docs/ai/CHANGELOG.md`.

This process must not change product, architecture, domain, engineering-standard, WORK, decision-record, code, test, migration, or current-state truth as a side effect.

## Resume in a new session

```text
Resume the Playhoot AI Workflow Change process from:
docs/ai/workspaces/active/<process-topic>/
```
