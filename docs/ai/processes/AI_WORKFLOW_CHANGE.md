# AI Workflow Change

Use this when you want to change Playhoot's AI-assisted development workflow.

Surface flow: CONVERSATIONAL AI <-> HUMAN for proposal/approval. CODEBASE AGENT applies the approved workflow mutation. CONVERSATIONAL AI/HUMAN perform the final human-facing result acceptance.

## When to use it

- You want to change AI execution surfaces, roles, authority, review gates, process routing, handoff prompts, workflow artifacts, templates, lifecycle semantics, or documentation synchronization behavior.
- You are fixing a material ambiguity in how the human, Conversational AI, Codebase Agent, or Independent Reviewer should work.
- The change is about the AI workflow itself, not product behavior or Playhoot system architecture.

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
Playhoot AI Workflow Change

Requested workflow change:
[describe it]

Problem with the current workflow:
[describe it]

Desired outcome:
[describe it]

Files or prompts I think may be affected:
[optional]
```

## Step 2 - CONVERSATIONAL AI

For editorial fixes, the AI may make the correction directly.

For material workflow changes, the AI will prepare `docs/ai/workspaces/active/<workflow-topic>/HUMAN_REVIEW.md` with the current workflow problem, impact map, minimal proposal, compatibility notes, validation plan, and decisions needed from you. If the workspace needs to be persisted and the Conversational AI cannot write to the repository, it will give you a CODEBASE AGENT HANDOFF.

## Step 3 - HUMAN

Review the proposal and respond with one of:

- approve the workflow change;
- reject it;
- request modifications;
- defer it;
- ask questions.

Material workflow changes require explicit human approval before they are applied.

## Step 4 - CODEBASE AGENT

After approval, applying the change to the affected AI workflow artifacts, validating references and routing, performing conceptual dry runs, and recording the change in `docs/ai/CHANGELOG.md` is a CODEBASE AGENT step, normally reached through a CODEBASE AGENT HANDOFF from the Conversational AI.

This process must not change product, architecture, domain, engineering-standard, WORK, decision-record, code, test, migration, or current-state truth as a side effect.

## Step 5 - CONVERSATIONAL AI / HUMAN

Once applied, the result is presented back for final human-facing acceptance.

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

Resume the Playhoot AI Workflow Change process from:
docs/ai/workspaces/active/<process-topic>/
```
