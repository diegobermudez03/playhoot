# SESSION-ADR-0027: Game Version Content Lives In Private Object Storage, Delivered To The Browser By Short-Lived Signed URLs

Status: ACCEPTED
Created: 2026-09-30
Last status change: 2026-09-30
Supersedes: None
Superseded by: None

## Context

`session/docs/GAME_VERSION_ARTIFACT_MODEL.md` (accepted design, implemented by `session_game_version_artifacts`) stores a Game version's `BackendScript` and `FrontendScript` as inline `TEXT` columns, and models assets as a JSONB list of `{Key, Kind}` with no place for the bytes themselves. Nothing in the repository stores or serves asset bytes, and nothing serves the frontend script to a browser.

Three facts make the inline shape a poor long-term fit:

- Scripts and assets are immutable, potentially large blobs. They are content, not relational state, and a relational database is not the right store for them.
- The frontend script and assets must reach a browser. Something has to decide how, without giving generated game code any network capability (`session/docs/FRONTEND_IFRAME_CONTRACT.md`).
- `SESSION-ADR-0008` already established, for archival, that a stable provider/key/object identifier is preferred over persisting an expiring or public URL. That principle applies equally here; this record extends it to live game content.

## Decision

### Content lives in private object storage; the database stores only locators

Backend script, frontend script and assets are stored as immutable objects in a private object-storage bucket. Session Runtime's database keeps only what is needed to obtain and verify them: an object key, a SHA-256 content hash, a size, and (for assets) a MIME type and the game's logical asset key. No URL is ever persisted.

Objects are content-addressed by SHA-256, so a published version references immutable content and identical bytes deduplicate physically. A hash is verified on every read; a mismatch is a hard failure with a monitoring alert, never a silent fallback.

This covers all three artifact parts, including `BackendScript`. The backend script never reaches a browser: Session Runtime reads it from storage and sends it inline to the Executor exactly as before, so `ADR-0016` and the Executor's wire contract are unchanged.

### Session Runtime depends on a port, not on a provider

Session Runtime depends on a narrow `ObjectStore` port (`Get`, `Put`, `PresignGet`). The production adapter uses Google Cloud Storage natively (`cloud.google.com/go/storage`); an in-memory fake serves tests. Configuration and authentication are GCS-specific rather than pretending every object store is interchangeable. Moving to another provider later means writing a new adapter, not changing Session Runtime.

Authentication uses Application Default Credentials and the deployment's service identity (Workload Identity where deployed on Cloud Run or GKE). Signed URLs are produced through Google-managed signing (IAM Credentials `signBlob`). No service-account private key is stored in configuration or secrets unless it is demonstrably unavoidable, and that exception requires its own decision.

### Bytes reach the browser by short-lived signed URLs, not through Session Runtime

To serve the frontend script or an asset, Session Runtime authorizes the caller as a participant of the Session, resolves the locator of the exact version the Session is pinned to, and issues a short-lived read-only signed URL for that single object, returned with the stored hash, content type and size. Playhoot's own trusted host frontend fetches directly from storage and verifies the hash. Session Runtime does not proxy or stream the bytes.

The trust boundary is unchanged and is the reason this is safe: **generated game code has no network capability and never receives a storage URL** (`FRONTEND_IFRAME_CONTRACT.md`). Only Playhoot's trusted host frontend holds signed URLs, and it hands the iframe a safe handle (`Blob`, `ArrayBuffer` or equivalent) through `playhoot.requestAsset`. The original constraint was never "no browser code may know a URL"; it was "untrusted generated code must not have arbitrary network capability."

### What a signed URL does and does not protect

A signed URL protects storage access control: an object is readable for a bounded time by whoever holds the URL. It is a bearer credential until it expires and cannot be revoked. It is not content secrecy or DRM: a participant authorized to receive bytes can keep them, and the frontend script is by definition inspectable by anyone who runs it. A short time-to-live, a private bucket and single-object scope bound the exposure.

## Rationale

Proxying every byte through Session Runtime would add bandwidth, CPU, connection and latency cost, and waste storage/CDN capabilities, to solve a problem that does not exist: hiding a URL from the trusted frontend is not a requirement. Public or permanent links would lose the authorization and pinned-version check entirely. Short-lived signed URLs keep authorization at issuance, keep the bucket private, and keep bytes off Session Runtime.

Externalizing the backend script adds a storage dependency to gameplay: a storage outage now stops actions on a cold cache, where previously only PostgreSQL mattered. This is an accepted trade-off, mitigated by an immutable hash-keyed cache (immutability makes it trivially safe) and by reading the locator before opening a transaction so the session row lock is not held across a storage round trip. It is reduced, not eliminated.

## Alternatives Considered

### Keep scripts inline in PostgreSQL, externalize only assets

Rejected by explicit human decision. Considered as the lower-risk option because the backend script is read on the hot path, but a single storage model for all versioned content was preferred, with the latency and availability trade-off accepted and mitigated as above.

### Session Runtime proxies or streams all bytes

Rejected. See Rationale. Reserved for special cases only if one appears.

### Public or permanent asset links

Rejected. They bypass authorization and the pinned-version check, and would be shareable indefinitely.

### S3-compatible adapter with generic configuration

Rejected in favor of a native GCS adapter. The `ObjectStore` port already provides provider independence; generic configuration would be portability in name only.

### CDN with signed URLs now

Classified LATER. Reevaluate when asset traffic becomes a measurable cost. The port and signed-URL model do not preclude it.

### Distinguish public published assets from private draft assets now

Classified LATER. Start with one model (private plus signed access) and optimize when there is a reason.

## Consequences

- `session_game_version_artifacts` drops `backend_script`, `frontend_script` and `assets`, gaining locator columns for each script; a new table holds one row per declared asset. Owned by `docs/projects/active/js-runtime-migration/works/WORK-0046-frontend-package-serving-and-versioned-asset-delivery.md`. No production data exists, so no backfill is required.
- Session Runtime gains its first external cloud dependency (`cloud.google.com/go/storage`) and its first infrastructure configuration (`GCS_PROJECT_ID`, `GCS_BUCKET`, `GCS_SIGNED_URL_TTL`).
- Deployment requirements for whoever provisions it: a private bucket, CORS allowing GET from the host frontend's origin, and a service identity able to read objects and to sign.
- Publishing content into the store is not part of this decision; it belongs to the future game-creation Project.
- `SESSION-ADR-0008`'s locator-over-URL principle is reaffirmed for live content. Its archival direction is unaffected; `SESSION-ADR-0023` still governs archive location.

## Canonical Knowledge Impact

- `session/docs/GAME_VERSION_ARTIFACT_MODEL.md` — dated addendum: scripts and assets stored as locators to private object storage.
- `session/docs/FRONTEND_IFRAME_CONTRACT.md` — serving mechanism and `requestAsset` resolution filled in; the four locked `playhoot.*` names and shapes untouched.
- `session/docs/DATA_MODEL.md` and `session/README.md` — updated when implemented.
- `session/docs/decisions/INDEX.md` — this record added.

## Implementation Impact

Routed to `docs/projects/active/js-runtime-migration/works/WORK-0046-frontend-package-serving-and-versioned-asset-delivery.md`. Not authorized beyond that WORK's own approved design.
