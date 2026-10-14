Process: Project execution (session-runtime-v1), human-directed implementation
Topic: Real-time Session Runtime per plans/session_runtime architecture plan
Current stage: tracking set up; waiting for the human's first implementation instruction
Current execution surface: CODEBASE AGENT
Related durable artifacts: docs/projects/active/session-runtime-v1/PROJECT.md, docs/projects/active/session-runtime-v1/CHECKLIST.md, plans/session_runtime/Playhoot_Session_Runtime_Architecture_Plan_with_TOC.docx, play/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md
Blocked by: nothing
Next action: wait for the human's instruction on what to implement first; map it to checklist IDs
Last durable checkpoint: Project, checklist and this file created (2026-10-13)
Last updated: 2026-10-13

## Working agreement (human decisions)

- The human directs what to implement and in what order. Do not plan a task order on your own.
- The agent debates, proposes, and flags where an instruction is under-specified or does not satisfy the plan's requirements; the human decides.
- The architecture plan is the skeleton. Concrete implementation is decided together, step by step.
- CHECKLIST.md is the end-state verification list. Tick items only when implemented AND verified. When the human's instructions skip a checklist item, surface it.
- Monitoring and metrics are mandatory for completion even though the plan lists them as deferred implementation detail (plan section 31).

## Context notes

- The plan lives only as a .docx. To read it: copy it to the scratchpad, unzip, and extract `word/document.xml` text. Promotion to durable owners is pending a human decision (see PROJECT.md).
- Domain directory is `play/` in the checkout although docs say `session/`.
- Existing helpers: `play/runtime/sandbox` (QuickJS/WASM worker), `play/session/internal/objectstore`, empty migrations registry. `monitoring/` has only `alerts.go` (no metrics yet). Logging standard: `docs/engineering/standards/logging.md`.
