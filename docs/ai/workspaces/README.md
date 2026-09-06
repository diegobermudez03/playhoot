# AI Process Workspaces

Status: TEMPORARY WORKFLOW CONTEXT

AI process workspaces live under:

`docs/ai/workspaces/active/<process-topic>/`

They are temporary, non-canonical, and non-authoritative.

They exist so a non-trivial AI process can survive chat/session boundaries without making chat history the only place where important context lives.

## Default Files

Use these files by default:

- `HUMAN_REVIEW.md`
- `AI_CONTEXT.md`

Do not create `docs/ai/workspaces/completed/`.

## HUMAN_REVIEW.md

`HUMAN_REVIEW.md` is for the human.

It should contain the current checkpoint or proposal in a form the human can understand and decide on.

Include only what is useful for the checkpoint, such as:

- process/topic;
- checkpoint;
- problem or desired outcome;
- recommendation or proposal;
- Mermaid diagrams when they help;
- contracts, API shape, schema, or examples when material to the decision;
- alternatives and tradeoffs;
- material decisions requiring approval;
- out-of-scope items;
- what happens if approved;
- next human action.

Do not fill it with generic AI mechanics, internal loading instructions, or template-authoring guidance.

## AI_CONTEXT.md

`AI_CONTEXT.md` is for future AI sessions.

It may contain:

- process name and stage;
- path to `HUMAN_REVIEW.md`;
- relevant canonical paths;
- evidence inspected;
- inherited constraints;
- human decisions already made;
- unresolved material decisions that are also visible in `HUMAN_REVIEW.md`;
- planning notes;
- likely touchpoints;
- validation or testing notes;
- pending system effects;
- next protocol action;
- resume context.

Do not put hidden material decisions in `AI_CONTEXT.md`.

Do not include private chain-of-thought. Record conclusions, evidence, constraints, and planning context only.

Avoid duplicating canonical knowledge. Reference durable owners instead.

`AI_CONTEXT.md` is not implementation authority and does not weaken Codebase Agent autonomy.

## Surface Ownership

A Conversational AI normally owns the design/conversation process and the content of `HUMAN_REVIEW.md`/`AI_CONTEXT.md`.

When the current Conversational AI cannot write to the repository, it produces a Codebase Agent Handoff (see `docs/ai/OPERATING_MODEL.md`) asking a Codebase Agent to persist or update these workspace files. Performing that persistence step does not give the Codebase Agent authority over the material decision being recorded — it remains whatever the Conversational AI/human already decided.

## Cross-Session Rule

Before asking the human for a material decision in a non-trivial process, persist enough context in the workspace for a fresh AI agent to resume.

When the human makes a material decision, persist the semantic result before relying on future chat memory.

If the decision changes accepted product, architecture, domain, engineering-standard, workflow, or implementation authority, synchronize the durable owner required by that process. The workspace is not enough.

## Promotion And Cleanup

Workspaces are not permanent archives.

Once the durable handoff point exists and relevant information has been promoted to canonical or durable owners, remove the temporary workspace.

Examples of durable owners include accepted product/architecture/domain/standard documentation, decision records, WORK specifications, current-state documentation after implementation, and the Engineering Radar where appropriate.

Historical memory belongs in those owners and Git history, not in completed workspace copies.
