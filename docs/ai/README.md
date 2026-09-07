# Working with AI on Playhoot

Just talk to the AI.

You normally do not need to choose a Playhoot process, know which internal
process comes next, manually orchestrate transitions between them, or
remember where unfinished work stopped. Paste the prompt below, describe
naturally what you want, and the Conversational AI figures out the rest.

## The Normal Entry Point

Paste into a CONVERSATIONAL AI:

```text
Execution surface:
CONVERSATIONAL AI

Repository:
https://github.com/diegobermudez03/playhoot

Repository ref:
<branch or commit>

Repository bootstrap:
Open the repository at the exact ref above and read `/AGENTS.md` first.
Follow its routing instructions to load only context relevant to my request.
Do not scan repository documentation indiscriminately. If you cannot access
the repository/ref or a required file, tell me instead of reasoning as if you
had read it.

Request:
<describe naturally what you want to do>

Determine the appropriate Playhoot workflow/process yourself and guide me
through it. Do not ask me to choose a Playhoot process unless an ambiguity
itself requires a meaningful human clarification. Transition to other
internal processes when needed. When repository mutation or checkout-local
execution is required, give me the CODEBASE AGENT HANDOFF I should run. Do
not assume repository state you cannot access.
```

`main` is fine when current pushed `main` is what you want inspected. A
Conversational AI can only reason about repository state it can actually
access — if what matters is uncommitted or unpushed, push it first or route
the step through a Codebase Agent instead. A URL alone does not mean the AI
has browsing/repository access; if it cannot actually open the
repository/ref, it should tell you instead of continuing as if it had.

This is the normal way to work with Playhoot's AI workflow, whether you are
starting something new or continuing something already in progress.

## What Happens Next

- The AI loads Playhoot correctly and identifies the appropriate internal
  process for your request — it does not ask you to name one unless a real
  ambiguity needs your input.
- It first checks whether your request continues something already open,
  instead of quietly starting duplicate work.
- It guides you through the process, entering or returning from a subprocess
  when a different concern needs resolving first, and explains why rather
  than naming internal process labels.
- It remains a proactive technical partner: it challenges your framing,
  proposes alternatives you did not mention, and teaches concepts you may not
  know — see the Principal Engineer Contract in
  `docs/ai/OPERATING_MODEL.md`.
- It asks you only for material decisions.
- When a repository mutation or checkout-local action is needed, it gives you
  the exact CODEBASE AGENT HANDOFF to run.
- Once a process crosses into repository effects, or otherwise needs to
  survive beyond one session, the AI persists its state in the repository so
  a future session — yours or a fresh one — can resume it.
- After a decision or milestone, it tells you what meaningfully comes next
  instead of just stopping; you can also ask naturally at any time: "what's
  open?", "what am I working on?", "where did this stop?", "continue."

## Human Authority

You approve material product, architecture, domain, engineering-standard,
workflow, and READY implementation decisions.

The AI may recommend, challenge, draft, implement approved work, and prepare
temporary workflow context for a Codebase Agent to persist, but it must not
hide material decisions in agent-only notes.

## Advanced / Manual Entry

You do not need any of this for normal use. CONVERSATIONAL AI reasons and
decides with you, CODEBASE AGENT changes the repository, and INDEPENDENT
REVIEWER checks implementation independently — the Orchestrator handles that
distinction for you automatically (full model in
`docs/ai/OPERATING_MODEL.md`).

If you want to inspect or drive the workflow manually — for debugging, or
because you already know exactly which internal process you want — these
remain available as human-facing process guides and reference:

- Product question: `docs/ai/processes/PRODUCT_DISCUSSION.md`
- System-level technical decision: `docs/ai/processes/ARCHITECTURE_DISCUSSION.md`
- Unfamiliar technical area where AI should lead: `docs/ai/processes/GUIDED_TECHNICAL_EXPLORATION.md`
- Domain or bounded-context ownership: `docs/ai/processes/DOMAIN_DESIGN.md`
- Concrete feature from design to implementation: `docs/ai/processes/FEATURE_DEVELOPMENT.md`
- Reusable coding, design, or engineering rule: `docs/ai/processes/ENGINEERING_STANDARD.md`
- Broad proactive technical review: `docs/ai/processes/PRINCIPAL_ENGINEER_REVIEW.md`
- Change this AI workflow itself: `docs/ai/processes/AI_WORKFLOW_CHANGE.md`

`docs/ai/workspaces/active/<initiative>/HUMAN_REVIEW.md` is where the AI
prepares your review whenever it needs a decision from you, whichever process
you (or the AI) ended up in. Its paired `AI_CONTEXT.md` is temporary AI
working memory, not hidden decision authority.

You do not need to read `docs/ai/protocols/`, `docs/ai/OPERATING_MODEL.md`,
`docs/ai/KNOWLEDGE_MAP.md`, templates, or `AI_CONTEXT.md` unless you want to
inspect how the workflow works internally.
