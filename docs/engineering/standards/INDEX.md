# Engineering Standards

Status: CANONICAL INDEX

This index routes engineers and AI agents to accepted reusable engineering standards.

- Load only standards relevant to the current task.
- Absence of a standard does not allow an AI to invent one.
- Use `docs/ai/processes/ENGINEERING_STANDARD.md` to create or change a reusable standard.
- Global architecture rules remain owned by `ARCHITECTURE.md`; do not duplicate them here.
- Implementation patterns observed in code are not canonical merely because they exist.

| Concern | Standard |
| --- | --- |
| Repository/query implementation, schema-aware repositories, multi-table persistence, shared-vs-local persistence extraction, transaction ownership, repository naming | `repositories.md` |
| Error propagation and error logging boundaries; expected business outcome vs. error classification | `error-handling.md` |
| Unexpected data/state integrity failures, alerts, and panic semantics | `data-integrity.md` |
| Unit/service/repository testing | `testing.md` |
| Domain logic placement, package organization, type ownership, workflow vs. use case, behavior locality, workflow controller shape, business/lifecycle-policy vs. data-integrity vs. persistence-mechanics responsibility split | `domain-logic-placement.md` |
| Persisted cross-domain entity-reference naming | `cross-domain-reference-naming.md` |
| Method/function contract conventions: explicit parameters vs. input structs, semantic primitive types, return-value shape | `function-signatures.md` |
| Natural vs. request/command idempotency, required tokens, replay/conflict semantics, replayed-outcome-vs-error distinction, claim mechanism contract, idempotency responsibility ownership | `idempotency.md` |
| Source-code comment purpose and style: what comments should/should not explain, per-symbol-kind guidance, task/history comments, comment-only refactors | `code-comments.md` |

This initial set was extracted from the previously canonical `AGENTS.md -> Project Patterns`.
