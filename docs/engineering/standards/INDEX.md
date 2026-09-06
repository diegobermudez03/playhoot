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
| Repository/query implementation | `repositories.md` |
| Error propagation and error logging boundaries | `error-handling.md` |
| Unexpected data/state integrity failures, alerts, and panic semantics | `data-integrity.md` |
| Unit/service/repository testing | `testing.md` |

This initial set was extracted from the previously canonical `AGENTS.md -> Project Patterns`.
