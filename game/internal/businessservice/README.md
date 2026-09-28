`businessservice` exposes methods for business validation and business behavior.

This pkg shouldn't handle any specific operation with state (data persisting) or transport layer, it just exposes methods specific for the business logic in this pkg, so any use case can use it

This package predates and is not the canonical pattern for domain logic placement — see `docs/engineering/standards/domain-logic-placement.md` and `ARCHITECTURE.md -> Intra-Domain Responsibility Boundary`. It migrates opportunistically when touched; no repository-wide refactor is authorized by that decision.
