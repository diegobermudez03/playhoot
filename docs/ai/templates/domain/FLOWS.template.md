# <Domain Name> - Flows

Status: CURRENT IMPLEMENTATION

Purpose: show important implemented domain behavior without forcing the reader to navigate Go implementation details.

This document is visual-first: use diagrams before explanatory text.

## <Flow Name>

```mermaid
sequenceDiagram
    participant Actor
    participant Domain
    participant Store

    Actor->>Domain: request
    Domain->>Store: persist/read state
    Domain-->>Actor: result
```

Notes:

- <only when the diagram cannot communicate an important invariant/failure behavior>

State diagrams may be used for important entity/session lifecycles. Flowcharts may be used when they communicate behavior more clearly.

If the domain currently has no meaningful implemented flows, this document may simply state that none are implemented yet.

## Rules

- Only document IMPLEMENTED behavior.
- Do not draw a target/future flow as current.
- If a flow is only partially implemented, either show only the implemented portion and label it PARTIAL, or omit it and represent its status in `CURRENT_STATE.md`.
- Prefer business/application-level flow steps.
- Do not diagram every private helper or function call.
- Show persistence interaction when it is important to understanding the behavior.
- Show supporting subsystems when they materially explain the flow.
- Cross-domain interactions may appear only if they actually exist and are consistent with canonical architecture.
- Keep prose minimal.
- Do not duplicate the same information already obvious from the diagram.

Use diagram first and minimal explanatory text second. Avoid large prose descriptions, duplicating code, describing implementation line-by-line, or drawing future architecture as current state.
