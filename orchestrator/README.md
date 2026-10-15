# Orchestrator

Orchestrator is the coordination layer for cross-domain write workflows.

- Coordinates write workflows across business domains.
- May persist workflow state when a workflow requires it.
- May manage retry, compensation, idempotency, and progression when a workflow requires them.
- Contains workflow coordination logic.
- Does not own the underlying business rules of participating domains.

Domains expose operations that protect their own responsibilities. Orchestrator coordinates how those operations are used in cross-domain workflows.

This README does not canonize a universal orchestration, choreography, transport, event, or queue mechanism.

## Organization: by behavior, not by domain

By nature an orchestration crosses domains, so it is not grouped by domain. Operations are grouped by the **behavior** they coordinate, one subpackage per group, so a caller can find an operation by asking "what is this about" rather than "which domains does it touch":

```text
orchestrator/
  doc.go                  # the package itself holds no code
  playlifecycle/          # taking a Game to being played as a Session (create a Session, ...)
    manager.go            # Manager: the group's controller and its dependencies
    create_session.go     # one operation: its own narrow interfaces + the method
```

- One subpackage per behavior group, named for the behavior (`playlifecycle`), never for a domain.
- One controller per group (`Manager`), holding the collaborators that group's operations need. There is deliberately no single `Orchestrator` struct that every group hangs off: it would accumulate every domain's dependencies and become a god object.
- Each operation lives in its own file, with the narrow interfaces it needs declared there. Those interfaces are unexported (nobody else imports them), and are mocked for the group's unit tests.
- A caller (today the `api` route groups) depends on the specific group it uses, declaring its own narrow interface for it.

## Types: simpler than a domain

The contract/implementation split a domain uses (a root package of exported types, implementation under `internal/`) is not needed here. Orchestrator is only ever imported by callers, never by another domain, so it cannot take part in an import cycle.

- Orchestrator may define its own input and result types in the group package when an operation needs them, for example a result composed from the values several domains returned.
- When a single domain's type already describes the result, prefer returning it directly instead of defining a new one.
- Orchestrator is the one place allowed to expose methods whose signatures use other domains' types. It is a gateway: it forwards, and being explicit about those types is useful to its callers.

## Depends only on the domains it calls

Orchestrator (like Composer) is the only layer allowed to depend on other domains for its interfaces, but only on the domains it actually calls. It never takes a type from a domain it does not talk to: `playlifecycle` does not call Identity, so it does not import it; the caller's user id is simply a string it forwards.

- **Plain strings by default.** An identifier or token is a `string` unless a named type would carry behavior (a validated format, say). Do not introduce a named string type just to label a value, and do not convert between two names for the same string.
- **A named type belongs to the domain that needs it.** If a domain decides a value is more than a string, that domain exports the type, and it is only used where that domain is called.
- **Domains stay isolated.** A domain's methods take plain values or its own types, never another domain's. When data from one domain goes to another (for example an Identity profile), the receiving domain defines its own type for what it cares about, and the caller, here the orchestrator, maps between them. The same entity does not mean the same thing in every domain.

## Current state

One group exists, `playlifecycle`, with one operation: `CreateSession`. It asks Game Management whether the Game is playable for the user (a yes/no answer); if not, it returns the `game_unavailable` outcome without creating anything, otherwise it calls the Session domain to create the Session and returns that result as is (a Session-domain type, no new one). It holds no state and passes the idempotency key through to the Session domain, which owns idempotency. Game Management's side is a placeholder that always answers playable, and the Session side is the real `play/session` creation, which answers `game_unavailable` until some flow registers games with an executable version (see the placeholders list in the Session Runtime project checklist).

See `../ARCHITECTURE.md` for global architecture rules.
