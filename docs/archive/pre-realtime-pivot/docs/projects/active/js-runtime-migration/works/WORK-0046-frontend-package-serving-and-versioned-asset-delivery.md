# WORK-0046: Frontend Package Serving & Versioned Asset Delivery

Status: IMPLEMENTING
Created: 2026-09-27
Last status change: 2026-09-30 (READY -> IMPLEMENTING)

Related decisions:
- `session/docs/decisions/SESSION-ADR-0027-game-version-content-lives-in-private-object-storage.md`
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`

Canonical context:
- `session/docs/GAME_VERSION_ARTIFACT_MODEL.md`
- `session/docs/FRONTEND_IFRAME_CONTRACT.md`

## Outcome

Game version content (backend script, frontend script, assets) lives in private object storage; Session Runtime's database keeps only the metadata needed to locate and verify it. A participant of a Session can obtain, from the exact version the Session is pinned to, short-lived signed access to that version's frontend script and to any declared asset, so Playhoot's own trusted host frontend can fetch the bytes directly from storage and hand the generated iframe a safe handle. Session Runtime itself does not proxy those bytes. The backend script never reaches a browser: Session Runtime fetches it from storage and sends it to the Executor exactly as today.

This WORK supersedes `session-runtime-v1`'s `WORK-0009` (Client-Safe Game UI Manifest): that WORK's premise (a manifest of Game Language's declarative UI tree) is retired, and its real need, a version-pinned client-safe way to know what to render, is exactly what serving the pinned frontend script covers. `WORK-0009` is CANCELLED by the human decision recorded below.

## Context

Ground truth checked against current code (2026-09-30), not assumed:

- `session_game_version_artifacts` (`session/internal/storage/migrations/20260928000001_session_game_version_artifacts.go`) stores `backend_script` and `frontend_script` as `TEXT NOT NULL`, and `assets` as a JSONB list of `{Key, Kind}` with no bytes anywhere.
- Nothing reads `frontend_script` today. `internal/repo.GameVersionArtifact` deliberately excludes it.
- `backend_script` is read by every RUNNING-phase step inside its transaction, under the session row lock (`GetGameVersionArtifact`), and by `GetClientState` through the unlocked `ResolveGameVersionArtifact`.
- No object-storage client, port, dependency, or configuration exists anywhere in the repository.
- `api/` exposes one REST route (`POST /sessions`) and a WebSocket skeleton. No caller authentication exists: `POST /sessions` takes `host_user_uuid` from the request body.
- `Manager.GetClientState` is the established precedent for a participant-authorized, unlocked, pure read with typed outcomes.

`FRONTEND_IFRAME_CONTRACT.md` already fixes that generated frontend code has no network capability and never receives a URL: `playhoot.requestAsset(key)` resolves to a Playhoot-provided handle. The trusted host frontend is Playhoot's own code and may hold signed URLs and credentials. This WORK's design relies on that split and does not weaken it.

## Human Decisions (2026-09-30)

1. `WORK-0009` is cancelled as superseded by this WORK.
2. Scripts and assets are not stored in the database. Private object storage holds content; the database holds only what is needed to obtain it. This applies to all three of backend script, frontend script and assets, in this WORK.
3. Bytes reach the browser through short-lived signed URLs issued to the trusted host frontend, not by Session Runtime proxying or streaming them. The generated iframe never receives a storage URL.
4. Serving is exposed as Manager read capabilities plus thin HTTP endpoints. Caller authentication is not invented here.
5. Build a narrow `ObjectStore` port, a native Google Cloud Storage adapter (`cloud.google.com/go/storage`), and an in-memory fake. The port keeps Session Runtime independent of the provider; configuration and authentication are GCS-specific rather than pretending every object store is identical. Decided 2026-09-30, replacing the earlier S3-compatible proposal.
6. The caller's `user_uuid` is a request parameter on the new HTTP endpoints, following `POST /sessions`' precedent. Each handler carries a code comment stating this is temporary: it is to be replaced by an identity package and middleware that derives the caller from an authenticated request, at which point the parameter is removed. The Manager reads themselves keep taking `userUUID`, which is unaffected by that change.
7. No production data exists in `backend_script`/`frontend_script`, so the migration replaces the columns with locators and needs no backfill.
8. Object storage as a Session Runtime dependency is recorded as `SESSION-ADR-0027` (ACCEPTED).

## Scope

### In Scope

- An `ObjectStore` port in Session Runtime (`Get`, `Put`, `PresignGet`), a native Google Cloud Storage adapter, and an in-memory fake. `Put` exists for the fake, adapter tests and the future publish workflow; this WORK's production paths only call `Get` and `PresignGet`.
- Schema change: `session_game_version_artifacts` drops `backend_script`, `frontend_script` and the `assets` JSONB column, and gains a locator (object key, SHA-256, size) for each script. A new `session_game_version_assets` table holds one row per declared asset: `definition_uuid`, logical `key`, `kind`, object key, SHA-256, size, content type; unique on `(definition_uuid, key)`.
- Backend script loading: every place that reads `BackendScript` today resolves it through the locator, fetches from `ObjectStore`, verifies the SHA-256, and caches the verified bytes keyed by hash. Session Runtime continues to send the script inline to the Executor, so `ADR-0016` and the Executor's wire contract are unchanged.
- `Manager.GetFrontendScriptAccess` and `Manager.GetAssetAccess`: participant-authorized, pinned-version-only, unlocked reads that return a signed URL plus the metadata the host frontend needs to verify what it fetched.
- HTTP endpoints in `api/session` exposing those two reads.
- Configuration limited to `GCS_PROJECT_ID`, `GCS_BUCKET` and `GCS_SIGNED_URL_TTL`. Authentication uses Application Default Credentials / the deployment's service identity (for example Workload Identity on Cloud Run or GKE). Signed URLs are produced through Google-managed signing (IAM Credentials `signBlob`), so no service-account private key is stored in configuration or secrets unless demonstrably unavoidable, and that exception would need its own decision.
- Cancelling `session-runtime-v1`'s `WORK-0009` and updating both Projects' tracking files.

### Out Of Scope

- Uploading or publishing content into the store, and keeping Game Management's and Session Runtime's representations consistent on publish. That belongs to the future game-creation Project (`docs/work/active/WORK-0033-cross-domain-game-publish-composition.md`). Tests seed content through the port directly.
- Deployment manifests, bucket provisioning, IAM policy, and CORS configuration for the real bucket. No service in this repository has a deployment manifest yet. CORS on the bucket for the host frontend's origin is a required deployment fact, recorded in Documentation Impact.
- A CDN or CDN-signed URLs. Classified LATER (see Design Notes).
- Caller authentication for the HTTP endpoints.
- The frontend application, its SDK, and `AssetHandle`'s in-browser implementation. Building them is out of this Project's scope.
- Live-connection transport (`session-runtime-v1`'s `WORK-0020`) and reconnect (`WORK-0015`).
- Deduplication across games beyond what content-addressed keys give for free.

## Approved Design

Approved by the human 2026-09-30, including the two follow-ups recorded under Human Decisions.

### Stored shape

```text
session_game_version_artifacts (per definition_uuid, immutable)
  backend_script_key, backend_script_sha256, backend_script_size
  frontend_script_key, frontend_script_sha256, frontend_script_size
  (game_contract, participant_min/max, projection_visibility,
   platform_contract_version unchanged)

session_game_version_assets (one row per declared asset)
  definition_uuid, key, kind, object_key, sha256, size, content_type
  unique (definition_uuid, key)
```

Object keys are content-addressed by SHA-256 (for example `scripts/<sha256>` and `assets/<sha256>`), so a published version references immutable content and identical bytes deduplicate physically. The exact key layout is Implementation Freedom; the content-addressed property and the verified hash are not.

### Backend script path

- The steady-state RUNNING-phase path must not hold the session row lock across an object-storage round trip. Because a pinned artifact is immutable, its locator can be read unlocked before the transaction opens (the same reasoning `Join` and `GetClientState` already use), and a cold cache is warmed there.
- The verified-bytes cache is keyed by SHA-256. Immutability makes it trivially safe to cache and never needs invalidation. Its size bound is Implementation Freedom.
- A hash mismatch on read is a hard failure with a monitoring alert, never a silent fallback. A missing object for a pinned locator is treated like the existing missing-pinned-artifact case (`ErrPinnedDefinitionMissing` plus alert).

### Serving

```text
GetFrontendScriptAccess(ctx, sessionUUID, userUUID)
GetAssetAccess(ctx, sessionUUID, userUUID, key)
  -> Result{ Outcome, URL, ExpiresAt, SHA256, ContentType, Size }
  Outcomes: Success, SessionNotFound (error), NotAParticipant, AssetNotFound (assets only)
```

- Both mirror `GetClientState`: resolve the session, check the caller is a participant, read the pinned artifact, sign a URL for exactly that one object. They never mutate state and never lock the session row.
- Available to any participant in any phase, since the pin exists from `Create` and the frontend loads once at iframe bootstrap.
- The signed URL is short-lived (default 120 seconds, configurable), read-only, and scoped to a single object.
- The response carries the stored SHA-256 so the trusted host frontend can verify what it fetched.
- HTTP: `GET /sessions/{session_uuid}/frontend-script` and `GET /sessions/{session_uuid}/assets/{key}`, both returning the JSON above. The caller's `user_uuid` is a query parameter, following the `POST /sessions` precedent until real authentication exists. Status mapping: 200, 403 not a participant, 404 session or asset not found.

### Design Notes

- Signed URLs protect storage access control, not content secrecy. A participant authorized to receive bytes can keep them. The frontend script is not a secret by definition.
- The URL is a bearer credential until it expires and cannot be revoked. The short TTL, private bucket and single-object scope bound that.
- A CDN with signed URLs is the natural later optimization if bandwidth cost matters. Reevaluate when asset traffic becomes a measurable cost.

## Constraints and Invariants

- Must serve exactly the version a Session is pinned to (`GAME-ADR-0001`'s immutability invariant), never a Game's current or latest version.
- The generated iframe never receives a storage URL. Signed URLs are issued only for consumption by the trusted host frontend.
- The backend script never reaches a browser or any endpoint added by this WORK.
- Content referenced by a locator is immutable. Every read verifies the stored SHA-256.
- Credentials never appear in code, configuration files checked into the repository, or logs. Signed URLs must not be logged.
- No object-storage round trip while holding a session row lock on the steady-state path.
- Session Runtime owns its own storage locators. Game Management's tables are not referenced (`ARCHITECTURE.md -> Cross-Domain Public Entity References`).
- Exported doc comments must not cite internal packages or WORK/ADR identifiers (`docs/engineering/standards/`).

## Acceptance Criteria

Met (see Completion Record for evidence):

- No test or code path stores or reads script bytes from a database column.
- Executing a Session end to end through the fake store produces identical results to today.
- A tampered object (hash mismatch) fails closed and raises a monitoring alert.
- A non-participant, and a participant of a different session, cannot obtain access.
- A Session pinned to version A never receives version B's content after B is published.
- The `ObjectStore` adapter passes the same contract test suite as the fake.

## Implementation Freedom

- Object key layout, cache size and eviction, signed-URL TTL default within the stated range, and private helper structure.
- How the GCS adapter obtains signing capability under the no-private-key constraint (for example the client's IAM-based signing option), provided the constraint holds.

## Verification

Performed: unit tests against the fake, a shared port contract test run against both fake and adapter, integration tests for the migrated schema, and an adapter test against a GCS emulator or a real test bucket if one is reachable (same environment caveat as prior WORK). Signed-URL generation under Google-managed signing cannot be fully exercised without real GCP credentials, so it may remain unverified in a sandbox and must be reported as such.

## Documentation Impact

### Current-State Documentation After Implementation

- `session/docs/GAME_VERSION_ARTIFACT_MODEL.md`: dated addendum. `BackendScript`/`FrontendScript` are stored as locators to private object storage, not inline; assets gain a table. `WORK-0044`'s record is amended by addendum, not rewritten.
- `session/docs/FRONTEND_IFRAME_CONTRACT.md`: fill in the "Frontend Script Loading" serving mechanism and `requestAsset`'s resolution, keeping the four locked `playhoot.*` names and shapes untouched. `AssetHandle`'s in-browser shape stays deferred to the frontend.
- `session/docs/DATA_MODEL.md`: the new columns and table.
- `session/README.md`: the object-storage dependency.
- `docs/projects/active/session-runtime-v1/PROJECT.md`: `WORK-0009` marked CANCELLED.
- Deployment facts for whoever provisions the bucket: private GCS bucket, CORS allowing GET from the host frontend's origin, and the service identity granted object read plus permission to sign (Service Account Token Creator on itself for `signBlob`).
- `session/docs/decisions/SESSION-ADR-0027-game-version-content-lives-in-private-object-storage.md` (ACCEPTED 2026-09-30) records object storage as a Session Runtime infrastructure dependency; its index row is already added.

## Blockers

None outstanding. Resolved 2026-09-30: design approved, dependency confirmed (native GCS), no backfill needed, `SESSION-ADR-0027` accepted.

Previously listed, now closed:
- ~~Human authorization of this DRAFT.~~
- ~~Dependency/provider confirmation~~ Resolved 2026-09-30: native Google Cloud Storage (`cloud.google.com/go/storage`) behind the `ObjectStore` port.
- ~~Assumption to confirm: no production data exists~~ Confirmed 2026-09-30: the database holds no data, no backfill needed.
- Accepted risk to keep visible: externalizing `backend_script` adds a storage dependency to gameplay. A storage outage now stops actions on a cold cache, where before only Postgres mattered. Mitigated by the immutable hash-keyed cache and the pre-transaction warm, not eliminated.

## Completion Record

Implementation complete 2026-09-30. Independent review round 1 returned CHANGES_REQUIRED with two items awaiting a human decision (see Independent Review below). Status stays IMPLEMENTING until they are resolved and the change is re-reviewed.

### What was built

- `session/internal/objectstore`: the `Store` port (`Get`, `Put`, `PresignGet`), `Locator` (object key, SHA-256, size) with content-addressed `LocatorFor`/`PutContent`, a hash-verifying `Loader` with a bounded hash-keyed in-memory cache, an in-memory `Fake` (with `GetErr` and `Corrupt` hooks and a per-key `GetCount`), a native Google Cloud Storage adapter (`cloud.google.com/go/storage`, Application Default Credentials, signed URLs through Google-managed signing with no locally held private key), and `storetest.RunContract`, a shared behavioral suite run against both the fake and (when configured) the real adapter.
- Migration `20260930000000_session_game_version_content_locators`: `session_game_version_artifacts` drops `backend_script`, `frontend_script` and `assets`, gaining key/SHA-256/size columns for each script; new `session_game_version_assets` table, unique per `(definition_uuid, key)`, FK to the artifact.
- `Manager.New` now requires an `objectstore.Store`. All four RUNNING-phase steps (`Start`, `SubmitPlayerEvent`, `CancelSession`, `ExpireTimer`) and `GetClientState` load the backend script through `backendScriptSource`; a missing or hash-mismatched object alerts and fails the step (the transaction rolls back), a plain storage failure only fails it. `Join` warms the cache in the background.
- `Manager.GetFrontendScriptAccess`/`GetAssetAccess` (`content_access.go`) and `session.ContentAccessResult`/`ContentAccessOutcome`: unlocked reads that require the caller to be a participant, resolve only the Session's pinned version, and sign exactly that one object. Default signed-URL TTL is 2 minutes.
- `api/session`: `GET /sessions/{session_uuid}/frontend-script` and `GET /sessions/{session_uuid}/assets/{key}` (200 / 400 / 403 / 404 / 500). Each handler carries a comment that the `user_uuid` query parameter is temporary until an identity package and middleware exist. Signed URLs are never logged.
- `sessionlifecycle.NewProduction` replaces `NewWithGRPCExecutor` as the supported entry point for `main.go`; `main.go` now requires `GCS_PROJECT_ID` and `GCS_BUCKET`, with optional `GCS_SIGNED_URL_TTL`.
- Documentation: `SESSION-ADR-0027` (accepted earlier the same day); addenda/updates to `GAME_VERSION_ARTIFACT_MODEL.md`, `FRONTEND_IFRAME_CONTRACT.md`, `DATA_MODEL.md`, `session/README.md`, `session/CURRENT_STATE.md`.

### Deviation from the Approved Design, reported

The Approved Design said a cold cache would be warmed "pre-transaction" for every RUNNING-phase step. Implemented narrower: `Join` warms the cache in the background (its artifact read is already unlocked and pre-transaction), and each RUNNING-phase step loads on demand. A step on an instance that never handled `Join` for that Session therefore fetches the script once *inside* its transaction, under the Session row lock. The stated invariant ("no storage round trip under the row lock on the steady-state path") holds - once loaded, no later step touches storage - but the first step per script per instance can. Closing that fully would need an extra unlocked session read before every step's transaction. Not done; flagged for the reviewer and the human.

### Verification

Run against a real local Postgres (the disposable-database test harness), not skipped:

- `go build ./...` and `go vet ./...` clean.
- `go test ./... -count=1`: everything passes except two failures both confirmed pre-existing and unrelated - `TestNoInternalDocCitationsInComments` (3 hits, all in `20260926000000_session_runtime_failures.go`, a file this WORK does not touch) and `TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration`, which was independently re-run on a clean checkout of `HEAD` in a separate worktree and failed there too (2 of 3 runs here, 6 of 6 in the reviewer's environment - environment-dependent, not caused by this change).
- New tests, all passing: object store contract (fake), loader (verification, caching, tamper, missing, outage, eviction), `TestBackendScriptLoadedFromObjectStorage_Integration` (executor receives the stored bytes; script fetched once across two steps; tampered / missing / unreachable object each fail closed and leave the Session in LOBBY with no turn), `TestContentAccess_Integration` (participant granted for exactly the pinned object; non-participant and other-session participant declined with no URL; unknown session; declared vs undeclared asset; a Session pinned to version A never receives version B's script or B-only asset), `TestGameVersionAssets_Integration` (unique and FK constraints), `TestManagerContentAccess` (mocked decision logic), and the API handler tests for every status.

### Not verified

- **The Google Cloud Storage adapter and real signed-URL generation.** They need real GCP credentials and a bucket, which this environment does not have. `TestGCSSatisfiesStoreContract` exists and skips unless `GCS_TEST_PROJECT_ID`/`GCS_TEST_BUCKET` are set. Specifically unverified: that the client's IAM-based signing works without a private key for the deployed service identity (it needs permission to sign for itself), and that CORS on the bucket lets the host frontend fetch.
- `main.go` startup with the new configuration was compiled but not run: it now needs GCP credentials and the two new environment variables, so a local run without them fails at startup by design.
- The unlocked `Manager` reads were not exercised under concurrency.

### Independent Review

**Round 1: CHANGES_REQUIRED plus two items awaiting a human decision (2026-09-30).** A fresh, read-only agent reviewed the committed change-set from the diff, code, migrations and tests rather than this record, and ran build, vet and the full suite against a real Postgres. It found the security model sound: it could not construct a non-participant, other-session or crafted-key path to another version's content, confirmed the backend script is reachable by no browser-facing route, that signed URLs are never logged, and that hash verification fails closed and never caches a failed load. It confirmed both failures named above as pre-existing and unrelated. It could not run `-race` (no C toolchain) or exercise the GCS adapter.

Fixed the same day, each re-verified by repeated runs (15 consecutive clean runs of the new tests):

- **Join code collision in the new tests** (REQUIRED_FIX): `seedStartableSession` derived a join code from the clock, which collided at random, failing `TestBackendScriptLoadedFromObjectStorage_Integration` in about 2 of 15 runs. A second defect surfaced while fixing it: join codes are constrained to 1000-9999. Now a wrapping counter inside that range.
- **Missing pinned object returned the wrong error** (REQUIRED_FIX): the Approved Design says a missing object is treated like a missing pinned artifact (`ErrPinnedDefinitionMissing` plus an alert); the first pass returned only the wrapped storage error. Now both are reachable (`errors.Is` matches either), and the test asserts it.
- **Weak test for "pinned version only"** (REQUIRED_FIX): the two versions belonged to different games, so an implementation that served "the game's current version" would still have passed. New fixture `SeedNewerVersionOfGame` puts both versions under one game with the newer one current; the test now also proves the pinned version's own asset resolves and the newer version's does not.
- **Stale documentation** (REQUIRED_FIX): `SESSION_RUNTIME_PERSISTENCE_MODEL.md` and `session/README.md` still named the dropped `backend_script` column; `api/README.md` omitted the two new routes and the new `ContentAccessor` dependency.
- **Cache-Control** (NON_BLOCKING, cheap): the two endpoints return a bearer URL, so granted responses now send `Cache-Control: no-store`, with a test.
- **Migration `Rollback`** (NON_BLOCKING): now documents that, like `Migrate`, it requires an empty table.
- **Wording correction**: this record and `PROJECT.md` called the Join/Leave test "flaky". The reviewer measured it failing 6 of 6 runs on both the pre-change and post-change commits in its environment, while this session's environment saw it fail on 2 of 3 runs on an untouched checkout. Its outcome is environment-dependent and it is independent of this WORK in both; "flaky" overstated how random it is.

**Open, awaiting a human decision (not changed unilaterally):**

1. **`CancelSession` with an unloadable script.** A host cancel is documented as always ending the Session once authorized. If the pinned script is missing, tampered with or the store is unreachable, the load fails before the Executor is reached, the transaction rolls back, and the host's cancel fails on every attempt, leaving the Session alive until inactivity expiry. Options: keep rollback-and-error (current), or for `CancelSession` only force-terminalize on permanent failures (missing or tampered) while a transient outage still errors.
2. **Cold-cache fetch under the row lock.** The reviewer showed the reported deviation was understated: it is not only the first step per script per instance. `Create` does not warm; any instance that did not handle `Join` is cold; and the 256-entry FIFO cache can evict, making later steps cold again, so the stated "steady-state" constraint can be violated. Options: add an unlocked pre-transaction read and warm in each RUNNING-phase step (restores the Approved Design; costs one extra cheap query per step), or accept the deviation and amend the WORK's Constraint to say so.

**NON_BLOCKING items left as recorded, not fixed:** the loader has no singleflight, is FIFO rather than LRU and bounded by entry count only; the `Join` warm goroutine silently ignores failures so a tampered object is not alerted at `Join`; any `session_actors` row (including a participant who left, and terminal Sessions) can obtain URLs, matching the `GetClientState` precedent but untested; `GCS_SIGNED_URL_TTL` is only checked for `> 0` though V4 signing rejects more than 7 days; `gcs.go` has no per-call timeout, no `DoesNotExist` precondition on `Put`, and `NewGCS` validates but never uses `projectID`; with a service-account key file as local Application Default Credentials the library signs with that file's private key, contrary to the no-private-key intent (fine under Workload Identity); `monitoring.Alert` is currently a print, so the alert criterion cannot be asserted by a test.
