# Orchestrator

Orchestrator is the coordination layer for cross-domain write workflows.

- Coordinates write workflows across business domains.
- May persist workflow state when a workflow requires it.
- May manage retry, compensation, idempotency, and progression when a workflow requires them.
- Contains workflow coordination logic.
- Does not own the underlying business rules of participating domains.

Domains expose operations that protect their own responsibilities. Orchestrator coordinates how those operations are used in cross-domain workflows.

This README does not canonize a universal orchestration, choreography, transport, event, or queue mechanism.

See `../ARCHITECTURE.md` for global architecture rules.
