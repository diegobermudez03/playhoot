# API

API is the external transport/application edge for Playhoot.

- Exposes user-facing transport endpoints.
- Translates transport requests and responses.
- May handle transport-level concerns.
- May perform BFF response shaping for frontend needs.
- May call a single-domain capability.
- May call Composer for cross-domain reads.
- May initiate Orchestrator workflows for cross-domain writes.
- Owns no business state.

BFF response shaping is not the same as cross-domain read composition. If a request requires combining multiple domain reads, that composition belongs to Composer.

This README does not define Identity or authorization ownership.

See `../ARCHITECTURE.md` for global architecture rules.
