# Orchestrator

Orchestrator is the coordination layer for cross-domain write workflows.

- Coordinates write workflows across business domains.
- May persist workflow state when a workflow requires it.
- May manage retry, compensation, idempotency, and progression when a workflow requires them.
- Contains workflow coordination logic.
- Does not own the underlying business rules of participating domains.

Domains expose operations that protect their own responsibilities. Orchestrator coordinates how those operations are used in cross-domain workflows.

This README does not canonize a universal orchestration, choreography, transport, event, or queue mechanism.

## Example: `CreateSession`

`Orchestrator.CreateSession` (`createsession.go`) is this package's first concrete workflow: it reads Game Management's visibility state for a `gameUUID` (`game/usecases/checkvisibility`), and only if the Game exists and is currently playable/visible does it call Session Runtime's `sessionlifecycle.Manager.Create`. A not-visible/not-found result short-circuits before Session Runtime is ever called, returning `session.ErrGameNotFound` unchanged - the same sentinel `Create` itself already returns for its own not-found case. This is a Cross-Domain Write (creating a Session), not a Cross-Domain Read, because it results in a persisted Session/JoinCode/idempotency-claim row - see `docs/projects/active/js-runtime-migration/works/WORK-0032-composer-mediated-session-creation-visibility-check.md`.

See `../ARCHITECTURE.md` for global architecture rules.
