# Working with AI on Playhoot

This is the normal human entry point for AI-assisted Playhoot work.

Most of the time you only need:

1. this file;
2. the relevant human-facing process under `docs/ai/processes/`;
3. `docs/ai/workspaces/active/<process-topic>/HUMAN_REVIEW.md` when the AI asks you to review a checkpoint.

You normally do not need to read `docs/ai/protocols/`, `docs/ai/OPERATING_MODEL.md`, `docs/ai/KNOWLEDGE_MAP.md`, templates, or `AI_CONTEXT.md` unless you want to inspect how the workflow works internally.

## Which AI do I use?

Playhoot's workflow is organized around capability, not product. You should never have to guess whether to paste something into a chat tool or a coding tool.

- CONVERSATIONAL AI -> discussion, reasoning, design, decisions.
- CODEBASE AGENT -> repository inspection/mutation, implementation, tests.
- FRESH CODEBASE AGENT / INDEPENDENT REVIEWER -> implementation review.

Every process below labels which surface each step requires. You should never have to infer "do I paste this into a chat tool or a coding tool?" on your own.

When you start or resume a process in a CONVERSATIONAL AI, include the repository identity plus a short bootstrap instruction so it knows what it can reason about and where to start reading:

```text
Repository:
https://github.com/diegobermudez03/playhoot

Repository ref:
<branch or commit>

Repository bootstrap:
Open the repository at the exact ref above and read `/AGENTS.md` first.
Follow its routing instructions to load only the context relevant to this task.
```

`main` is fine when current pushed `main` is what you want inspected. A Conversational AI can only reason about repository state it can actually access — if what matters is uncommitted or unpushed, push it first or route the step through a Codebase Agent instead. A URL alone does not mean the AI has browsing/repository access; if it cannot actually open the repository/ref, it should tell you instead of continuing as if it had.

You do not need to manually list every relevant Playhoot doc — `/AGENTS.md` and the repository's own routing decide what the AI loads for the task at hand. Every process prompt below already includes this bootstrap line, so you normally just fill in the process-specific fields.

## Process Menu

- Product question: `docs/ai/processes/PRODUCT_DISCUSSION.md`
- System-level technical decision: `docs/ai/processes/ARCHITECTURE_DISCUSSION.md`
- Unfamiliar technical area where AI should lead: `docs/ai/processes/GUIDED_TECHNICAL_EXPLORATION.md`
- Domain or bounded-context ownership: `docs/ai/processes/DOMAIN_DESIGN.md`
- Concrete feature from design to implementation: `docs/ai/processes/FEATURE_DEVELOPMENT.md`
- Reusable coding, design, or engineering rule: `docs/ai/processes/ENGINEERING_STANDARD.md`
- Broad proactive technical review: `docs/ai/processes/PRINCIPAL_ENGINEER_REVIEW.md`
- Change this AI workflow itself: `docs/ai/processes/AI_WORKFLOW_CHANGE.md`

## How Checkpoints Work

For short discussions, the AI may answer directly in chat.

For non-trivial or multi-session work, the AI will prepare a human checkpoint at:

`docs/ai/workspaces/active/<process-topic>/HUMAN_REVIEW.md`

That file is the one meant for you. It should explain the problem, recommendation, tradeoffs, material decisions, and what happens if you approve.

The paired `AI_CONTEXT.md` file is for future AI sessions. It is temporary working memory, not hidden decision authority.

## Human Authority

You approve material product, architecture, domain, engineering-standard, workflow, and READY implementation decisions.

The AI may recommend, challenge, draft, implement approved work, and maintain temporary workflow context, but it must not hide material decisions in agent-only notes.
