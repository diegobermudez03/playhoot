# AI Workflow Change Protocol

Status: AGENT EXECUTION PROTOCOL

Use with the human-facing guide at `docs/ai/processes/AI_WORKFLOW_CHANGE.md`. This protocol is primarily executed by a CONVERSATIONAL AI for the proposal/approval steps; applying an approved change to repository files is a CODEBASE AGENT step.

## Scope

This protocol modifies the AI-assisted development system itself: roles, authority, escalation behavior, review requirements, lifecycle/status semantics, process routing, workflow artifacts, handoff protocol, templates, and documentation synchronization behavior.

Do not use this meta-process as a backdoor for product, architecture, domain, engineering-standard, WORK, decision-record, implementation, test, migration, or current-state truth changes.

## HUMAN SURFACE

For material workflow changes, prepare `docs/ai/workspaces/active/<workflow-topic>/HUMAN_REVIEW.md` with:

- current workflow problem;
- current behavior and owner;
- impact map;
- minimal proposal;
- alternatives considered;
- compatibility/migration behavior;
- validation plan;
- material decisions requiring approval;
- what happens if approved.

For editorial corrections, a direct concise explanation is enough.

## SYSTEM EFFECTS

- Load relevant workflow sources through the Knowledge Map and affected files directly.
- Classify changes as EDITORIAL/NON-SEMANTIC or MATERIAL.
- Detect and report workflow drift before building a proposal on a false assumption.
- Apply human-approved material changes coherently across affected workflow artifacts.
- Respect One Fact, One Owner.
- Validate stale references, routing, lifecycle semantics, templates, protocols, and compatibility.
- Append a material workflow entry to `docs/ai/CHANGELOG.md`.
- Preserve existing active/historical artifact semantics unless the approved compatibility plan says otherwise.
- For non-trivial or multi-session workspaces, maintain the paired
  `docs/ai/workspaces/active/<workflow-topic>/AI_CONTEXT.md` alongside
  `HUMAN_REVIEW.md` and follow `docs/ai/workspaces/README.md`.
- Before asking for a material decision, ensure the workspace has enough
  persisted state for a fresh agent to resume. When the human answers, update
  the active process state and/or durable owner before future continuation
  depends on it, keeping the human-facing checkpoint current where practical.

No material workflow decision may exist only in `AI_CONTEXT.md`.

## Material vs Editorial

MATERIAL changes affect semantics such as human/AI authority, role responsibility, escalation behavior, review behavior, lifecycle/status semantics, process routing, required workflow artifacts, persistent artifact meaning, handoff protocol, mandatory process gates, documentation synchronization behavior, or workflow knowledge ownership.

EDITORIAL corrections include typos, grammar, formatting, broken links, and wording clarifications that do not change behavior or authority.

If doubtful, treat the change as material until clarified.

## Execution

1. Capture request, problem, and desired outcome.
2. Classify editorial vs material.
3. For material changes, establish current behavior, authority owner, routing, and persistence/lifecycle behavior.
4. Produce a workflow impact map.
5. Propose the smallest coherent change.
6. Obtain explicit human approval, including compatibility/migration behavior.
7. Apply approved workflow changes coherently.
8. Validate consistency and stale references.
9. Perform conceptual dry runs.
10. Present the result for human acceptance.

## Compatibility

Material workflow changes must consider active WORK, completed WORK, ADR/PDR records, Radar structure/items, domain docs generated from templates, and other persistent workflow artifacts.

Normally, historical terminal artifacts remain historical. Active artifacts require an explicit compatibility decision when persistent artifact semantics change.

Use strategies such as FUTURE ARTIFACTS ONLY or MIGRATE ACTIVE ARTIFACTS based on the specific approved change. Do not establish a universal migration rule.

## Dry Run

At minimum, conceptually exercise:

1. normal Feature Development;
2. an Architecture/Domain decision path;
3. one edge/failure scenario relevant to the workflow change.

Do not create fake persistent WORK/ADR/PDR records merely to dry-run.

## Completion

For a material workflow change, complete only when the current workflow problem was understood, impact was mapped, the human approved the change, compatibility was handled, workflow owners were updated coherently, stale references were checked, validation has no unresolved material contradiction, dry runs are coherent, no Playhoot non-workflow truth changed, and `docs/ai/CHANGELOG.md` contains the applied material change.
