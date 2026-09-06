# Principal Engineer Review

Use this when you want the AI to proactively inspect Playhoot for risks, gaps, adequate areas, and next-step recommendations.

Surface flow: CONVERSATIONAL AI performs/coordinates the human-facing review. A CODEBASE AGENT may be used for repository-local evidence when required, and to persist Radar updates when the Conversational AI lacks write access.

## When to use it

- After a meaningful milestone.
- Before public launch.
- After major architecture changes.
- Approximately every 4-6 substantial features.
- Whenever you want broad Principal Engineer judgment rather than a narrow answer.

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
Playhoot Principal Engineer Review

Review trigger:
[milestone, concern, or timing]

Areas I especially care about:
[optional]

Desired depth:
[optional]
```

## Step 2 - CONVERSATIONAL AI

The AI will produce a Principal Engineer Review covering relevant risks, opportunities, adequate areas, and recommendations. Recommendations are usually classified as NOW, SOON, LATER, or NOT NEEDED.

For non-trivial reviews, the AI may prepare `docs/ai/workspaces/active/<review-topic>/HUMAN_REVIEW.md` with the review summary and proposed Radar updates. If repository-local evidence must be inspected, or the workspace/Radar needs to be persisted and the Conversational AI cannot write to the repository, it will give you a CODEBASE AGENT HANDOFF.

## Step 3 - HUMAN

Decide which recommendations deserve follow-up.

You do not need to approve every non-authoritative Radar update before it can be persisted, but material product, architecture, domain, standard, and implementation decisions still require their normal processes.

## Step 4 - CODEBASE AGENT

Meaningful recommendations may be synchronized into `docs/engineering/ENGINEERING_RADAR.md`, normally via a CODEBASE AGENT HANDOFF from the Conversational AI.

Radar items are not approved architecture, standards, product roadmap, or implementation authority.

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

Resume the Playhoot Principal Engineer Review process from:
docs/ai/workspaces/active/<process-topic>/
```
