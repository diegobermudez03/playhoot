# AI Workflow Change

## Purpose

Modify this AI-assisted development system itself.

This process owns how material AI workflow changes are proposed, approved, applied, and validated.

## Use This When

- You want to change Codex autonomy, review requirements, documentation sync, process structure, AI prompts, or mandatory workflow artifacts.
- Examples: require threat modeling, add a new process, change runbook prompts, change bootstrap or knowledge routing.
- You want to change lifecycle/status semantics, handoff protocol, workflow persistence semantics, artifact templates, or the behavior represented by a reusable workflow template.

## Do NOT Use This When

- You are changing product behavior.
- You are changing architecture, domain ownership, or engineering standards for Playhoot itself.
- You are implementing a product feature.
- You are creating concrete implementation/tooling work.

Examples:

- Change Game bounded-context ownership -> `DOMAIN_DESIGN.md` / `ARCHITECTURE_DISCUSSION.md`.
- Choose a database technology -> `GUIDED_TECHNICAL_EXPLORATION.md` / `ARCHITECTURE_DISCUSSION.md` as appropriate.
- Create a reusable repository rule -> `ENGINEERING_STANDARD.md`.
- Implement a product capability -> `FEATURE_DEVELOPMENT.md`.
- Change how Codex receives an approved WORK -> `AI_WORKFLOW_CHANGE.md`.
- Change the statuses of a WORK specification -> `AI_WORKFLOW_CHANGE.md`.

Do not use this meta-process as a backdoor for modifying product or technical architecture.

## Starting Information

Provide the requested workflow change, the problem with the current workflow, desired outcome, and any affected runbooks or prompts.

## Start This Process

```text
We are using docs/ai/processes/AI_WORKFLOW_CHANGE.md.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve relevant AI workflow context. Inspect affected workflow files directly. Inspect code/tests/migrations only if current implementation matters to the workflow change.

Act under the Principal Engineer Contract. Challenge whether the workflow change is necessary, propose simpler or alternative workflow changes, teach tradeoffs, keep the change minimal and coherent, and make a recommendation.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report workflow drift. Do not modify product, architecture, domain, engineering-standard, WORK, decision-record, or implementation truth merely because the AI workflow changes.

Requested workflow change:
[paste request]
```

## Material vs Editorial Changes

A MATERIAL workflow change changes semantics such as:

- human/AI authority;
- role responsibility;
- escalation behavior;
- required review behavior;
- lifecycle/status semantics;
- process routing;
- required workflow artifacts;
- persistent artifact structure/meaning;
- handoff protocol;
- mandatory process gates;
- documentation synchronization behavior;
- workflow knowledge ownership;
- the behavior represented by a reusable workflow template.

Material changes MUST use this process.

An EDITORIAL / NON-SEMANTIC correction includes things such as:

- typo;
- grammar;
- formatting;
- broken link/path;
- wording clarification that does not change behavior or authority.

Editorial corrections may be applied directly. They do not require a full workflow-change cycle and normally do not receive a changelog entry.

If there is doubt whether wording changes semantics, treat it as material until clarified.

## Process Stages

1. REQUEST / PROBLEM.
2. Classify the change.
3. Current workflow model.
4. Workflow impact map.
5. Minimal coherent proposal.
6. Human approval.
7. Apply coherently.
8. Consistency validation.
9. Dry run.
10. Accept result.

## Stage 1: Request / Problem

Capture:

- requested workflow change;
- concrete problem with the current workflow;
- desired outcome.

The Design AI must not assume the requested mechanism is the correct solution.

Challenge:

- Does the problem actually exist?
- Is it recurring or material enough to change persistent workflow?
- Can the problem be solved with the current workflow?
- Is a documentation clarification enough?
- Is there a simpler change?
- What happens if we do nothing?

Possible outcome:

NO WORKFLOW CHANGE

is valid.

## Stage 2: Classify The Change

Determine whether the requested change is:

- EDITORIAL / NON-SEMANTIC; or
- MATERIAL.

If editorial, use the lightweight path. Do not manufacture an approval ceremony or changelog entry.

If material, continue through the full process.

## Stage 3: Current Workflow Model

Before proposing a material change, identify the CURRENT behavior.

Load only relevant workflow sources using `docs/ai/KNOWLEDGE_MAP.md` and direct affected artifacts.

State concisely:

- current behavior;
- current authority owner;
- current routing;
- current persistence/lifecycle behavior where relevant.

Do not infer current workflow from chat history when repository workflow artifacts exist.

If current workflow documents contradict one another, report:

```text
WORKFLOW DRIFT DETECTED

Artifacts:
...

Conflict:
...

Likely explanation:
...

Recommended resolution:
...
```

Do not build a new workflow proposal on an unresolved false assumption.

## Stage 4: Workflow Impact Map

Before approval, create a concise WORKFLOW IMPACT MAP.

It must consider, where relevant:

- authority / role ownership;
- process/runbook behavior;
- protocols;
- lifecycle/statuses;
- templates;
- persistent artifact semantics;
- human entry-point routing;
- machine Knowledge Map routing;
- handoff prompts;
- cross-process references;
- existing active persistent artifacts;
- historical/terminal artifacts;
- compatibility/migration needs.

Output conceptually:

```text
WORKFLOW IMPACT MAP

Behavior changing:
...

Canonical workflow owner(s):
...

Files expected to change:
- ...

Files expected to remain unchanged:
- ...

Existing persistent artifacts affected:
...

Compatibility / migration concern:
...

Cross-process references to validate:
...
```

Do not require empty categories when genuinely irrelevant.

## Active/Historical Artifact Compatibility

A material workflow change must explicitly consider artifacts created under the previous workflow.

Examples include:

- active WORK specifications;
- completed WORK specifications;
- ADR/PDR records;
- Radar structure/items;
- domain documentation generated from templates;
- other persistent workflow artifacts.

Do not assume changing a template automatically means rewriting all old artifacts.

Normally, historical terminal artifacts remain historical and are not substantively rewritten. Active artifacts may require an explicit compatibility decision.

For a change that affects persistent artifact semantics, determine an appropriate strategy such as:

- FUTURE ARTIFACTS ONLY;
- MIGRATE ACTIVE ARTIFACTS;
- another explicitly human-approved compatibility approach.

Do not establish a universal migration rule. Choose based on the specific change.

If the compatibility approach itself is material, the human approves it as part of the workflow change.

## Stage 5: Minimal Coherent Proposal

Design the smallest change that solves the actual workflow problem.

The proposal should state:

- behavior before;
- behavior after;
- why change is needed;
- alternatives considered when material;
- affected artifacts;
- compatibility/migration behavior;
- validation plan.

Avoid broad workflow redesign when a targeted change works.

Check for:

- duplicated authority;
- contradictory ownership;
- redundant process layers;
- unnecessary new statuses;
- unnecessary new files;
- unnecessary IDs/registries;
- unnecessary mandatory human checkpoints.

Apply the same anti-overengineering mindset used elsewhere in Playhoot.

## Stage 6: Human Approval

A MATERIAL workflow proposal has no authority until explicitly approved by the human.

The Design AI may recommend the change. Codex must not self-approve it.

Human approval includes material compatibility/migration behavior.

If the human requests revisions, update the proposal before applying.

## Stage 7: Apply Coherently

After human approval, update the affected workflow artifacts coherently.

Prefer one coherent repository change so the workflow is not intentionally left in a contradictory half-migrated state.

When a rule/authority moves:

- add/update the new canonical owner;
- remove stale duplicated semantics from the previous owner;
- update affected references/routing.

Do not solve contradictions by duplicating the same rule everywhere.

Respect One Fact, One Owner.

Do not rewrite unaffected workflow artifacts "for consistency" when their owned knowledge did not change.

## Stage 8: Consistency Validation

After applying a material workflow change, inspect the affected workflow surface.

Validate at minimum where relevant:

- no stale references to renamed/removed files;
- no stale references to removed statuses/verdicts/dispositions;
- no contradictory authority;
- no duplicated canonical workflow semantics;
- human README routing remains correct;
- Knowledge Map routing remains correct;
- templates match the process that creates/uses them;
- protocols match lifecycle semantics;
- examples/prompts do not encode the old behavior;
- active artifact compatibility was handled as approved;
- historical artifacts were not inappropriately rewritten;
- no product/architecture/domain/engineering-standard truth changed as a side effect.

Search repository references to materially changed file paths, status names, artifact names, protocol names, and key semantic terms when useful.

Do not mechanically scan/rewrite unrelated knowledge.

## Stage 9: Dry Run

Conceptually exercise the changed workflow before declaring it complete.

At minimum test:

1. A normal Feature Development path.
2. An Architecture/Domain decision path, even if only to verify that an unrelated change did not accidentally alter material-decision authority.
3. One edge/failure scenario specifically relevant to the workflow change.

For example:

- implementation discovers a material issue;
- review returns DECISION_REQUIRED;
- an active artifact created under the old workflow exists;
- a proposed status transition becomes invalid;
- a workflow artifact is missing.

The dry run should answer:

- Does the human know where to start?
- Does Design AI know what it owns?
- Does Codex know what it may decide?
- Is persistent knowledge routed correctly?
- Can the workflow recover from the relevant edge case?
- Is there any contradictory instruction?

Do not create fake persistent WORK/ADR/PDR records merely to dry-run.

Dry runs are conceptual unless a specific change requires safe temporary validation.

## Fresh Review For Material Changes

For a materially significant workflow change, prefer a fresh agent/session for the final consistency/dry-run review when practical.

The reviewer should reason from current workflow artifacts and the approved change, rather than trusting the modifying agent's summary.

This is not a new persistent review-artifact system.

Do not create:

- workflow review files;
- workflow review IDs;
- extra lifecycle statuses.

For a small semantic change, proportional validation is sufficient.

## Stage 10: Accept Result

After application, consistency validation, dry runs, and fixes to workflow issues discovered during validation, present the resulting workflow to the human.

The human remains final authority for accepting the material workflow result.

Do not require human approval for each typo discovered while applying the already-approved workflow change.

## Design AI Responsibilities

- Keep the workflow self-modifying safely.
- Avoid broad rewrites when a targeted change works.
- Check for stale references, contradictory prompts, duplicated authority, incomplete routing, template/protocol mismatch, and compatibility gaps.
- Preserve human authority over material decisions.
- Avoid turning Playhoot technical design decisions into AI workflow changes.

## Human Decision Checkpoints

- Confirm current workflow insufficiency.
- Approve the minimal coherent material change before implementation.
- Approve material compatibility/migration behavior.
- Accept results after consistency validation and dry run.

## Potentially Affected Artifacts

- `AGENTS.md`
- `docs/ai/README.md`
- `docs/ai/OPERATING_MODEL.md`
- `docs/ai/KNOWLEDGE_MAP.md`
- `docs/ai/CHANGELOG.md`
- `docs/ai/processes/*`
- `docs/ai/protocols/*`
- `docs/ai/templates/*`
- `docs/work/README.md`
- `docs/work/templates/*`
- `docs/decisions/README.md`
- `docs/decisions/templates/*`
- `docs/engineering/ENGINEERING_RADAR.md`

## Possible Outputs

- No workflow change.
- Editorial correction.
- Approved workflow change.
- Updated workflow files.
- Follow-up workflow change proposal.
- Workflow changelog entry for an applied material change.

## Persistence / Documentation Rules

Workflow changes belong in AI workflow artifacts only. Do not modify product, architecture, domain, engineering-standard, WORK, decision-record, or implementation truth merely because the AI workflow changes.

Pure workflow changes do not require a WORK specification. This process is the owning meta-process.

If a workflow proposal also requires actual implementation/tooling code, separate the concerns. Use this process for workflow semantics and route concrete implementation/tooling work through the appropriate Feature Development / WORK process when needed.

After an applied MATERIAL workflow change passes validation and is accepted, append/update the corresponding entry in `docs/ai/CHANGELOG.md`.

Normally the changelog entry should be part of the same coherent repository change as the workflow update.

Editorial changes normally do not receive a changelog entry.

## Completion Criteria

For a MATERIAL workflow change:

- Current workflow problem was understood.
- Impact map was performed.
- Human approved the material change.
- Compatibility/migration behavior was explicitly handled where relevant.
- Affected workflow owners were updated coherently.
- Stale affected references were checked.
- Consistency validation has no unresolved material contradiction.
- Dry runs are coherent.
- No Playhoot product/architecture/domain/standard truth was silently changed.
- Human accepted the resulting workflow.
- `docs/ai/CHANGELOG.md` contains the applied material change.

For an EDITORIAL change, use proportional validation and do not require the full material-change ceremony.

## Where To Go Next

- Product process affected: `PRODUCT_DISCUSSION.md`.
- Architecture process affected: `ARCHITECTURE_DISCUSSION.md`.
- Domain process affected: `DOMAIN_DESIGN.md`.
- Feature process affected: `FEATURE_DEVELOPMENT.md`.
- Standard process affected: `ENGINEERING_STANDARD.md`.
