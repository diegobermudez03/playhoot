`businessservice` exposes methods for business validation and business behavior.

This pkg shouldn't handle any specific operation with state (data persisting) or transport layer, it just exposes methods specific for the business logic in this pkg, so any use case can use it
