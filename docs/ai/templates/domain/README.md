# Domain Documentation Templates

These templates define the standard documentation shape for an accepted Playhoot business domain / bounded context.

Instantiate them only after the domain boundary has been accepted. Do not create domain documentation to legitimize an unresolved boundary. Resolve the boundary first through `docs/ai/processes/DOMAIN_DESIGN.md`.

folder != domain
package != domain

Template mapping:

- `DOMAIN_README.template.md` -> `<domain>/README.md`
- `CURRENT_STATE.template.md` -> `<domain>/CURRENT_STATE.md`
- `DATA_MODEL.template.md` -> `<domain>/docs/DATA_MODEL.md`
- `FLOWS.template.md` -> `<domain>/docs/FLOWS.md`

Authority:

- `<domain>/README.md` describes accepted domain design.
- `<domain>/CURRENT_STATE.md`, `<domain>/docs/DATA_MODEL.md`, and `<domain>/docs/FLOWS.md` describe current implementation.

Current-state documents must be verified against code, tests, and migrations. Keep documentation concise and visual-first. One fact should have one canonical owner.
