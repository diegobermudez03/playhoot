# SESSION-ADR-0034: Game Assets And Signed Access Shared Across Sessions

Status: ACCEPTED
Created: 2026-10-25
Last status change: 2026-10-25
Supersedes: None
Superseded by: None

## Context

An EXECUTABLE game version had one frontend package: a single self-contained HTML file (decided 2026-10-17 for v1, "assets resolved separately by Playhoot"). The frontend repository needs real games with images, sounds and video, which do not belong inside one HTML file.

Players reached the package through `session_started.frontend`: a URL to private object storage, signed for 15 minutes at the start and signed again for every connection attaching to a running game (reattach, rejoin, device switch, admin). In production, without a private key, the Google client signs through Google's IAM service, so every signature is an external call. With assets, every connection would need one signature per asset, and a URL signed for 15 minutes can expire while its Session is still being played.

Game authoring and publishing (uploads, DRAFT to EXECUTABLE) do not exist yet; versions come from seed migrations.

The human decided the design on 2026-10-25, asked from the frontend repository.

## Decision

1. **Assets by logical key.** A game version declares assets, each by a logical key with a type. Types are `image`, `audio` and `video` only; a sprite or sprite sheet is an image. Accepted MIME types are a closed list: `image/png`, `image/webp`, `image/jpeg`, `image/svg+xml`; `audio/mpeg`, `audio/ogg`, `audio/wav`; `video/mp4`, `video/webm`. One asset weighs at most 5 MB (image), 10 MB (audio) or 50 MB (video), and a version's assets 100 MB together (1 MB = 2^20 bytes, chosen in implementation). A key matches `^[A-Za-z][A-Za-z0-9_]{0,63}$`, because the game writes it as a key of a JavaScript object literal.
2. **Stored per version.** Assets live in `play_game_version_assets` (version, key, type, MIME type, object key, SHA-256, size; one key per version). The object key may be NULL: the row stays as a record of an asset whose object was deleted. The application enforces every asset rule (key pattern, type, MIME type and size for the type, hash format, total weight); the database only keeps one key per version (`docs/engineering/standards/database-constraints.md`, amended 2026-10-25 by the human). The application treats an EXECUTABLE version whose stored assets break the rules (for example, over 100 MB together) as broken data, like missing package locators or aspect ratio.
3. **Required rules for the future publishing flow (not built).** When a version becomes EXECUTABLE, a static analysis of its frontend script confirms that every asset it declares was uploaded. Assets uploaded but not declared are deleted from storage and keep their row, without object. An EXECUTABLE version is immutable, assets included.
4. **One batch of signed URLs per version, shared by its Sessions.** The backend keeps one current batch per game version: signed URLs to the frontend package and to every asset with an object, all signed together with one expiry. It lives in the database (`play_game_version_signed_urls`), so every instance shares it. There is no history; a new batch replaces the whole batch.
5. **Reuse or sign.** Whenever a Session needs frontend access (its start, and every connection attaching to its running game), the current batch is reused if it expires more than a margin (5 minutes, for clock skew) after that Session's durable `play_sessions.expires_at`. Otherwise a new batch is signed for the whole version, valid 24 hours, and stored. Concurrent signers are harmless: any valid batch works, and the one expiring latest is kept. The 24 hours must exceed the Session's maximum lifetime plus the margin and stay within Cloud Storage's V4 limit of 7 days; a test holds both. This replaces the 15-minute signing per connection.
6. **No refresh endpoint.** A URL handed to a client always outlives its Session. A reconnection gets `session_started` again, with the current batch.
7. **Wire.** `session_started.frontend` gains `assets`: `[{key, type, mime_type, sha256, size, url}]`, every asset of the version with an object (empty when none). `frontend.url` and `expires_at` come from the same batch, whose expiry the assets share. The rest of the message does not change. Admins get the same.
8. **Failures.** A failed signing keeps the existing fallback behavior: at the start it fails the start; for a connection attaching to a running game it keeps the access of the start.

## Rationale

- One signature per asset per connection does not scale with assets, and each is an external call in production. A batch per version turns that into one signing per version per day, shared by every Session of the version and every instance.
- Tying the reuse rule to the Session's own durable expiry means no client ever holds a URL that dies before its Session, so no refresh protocol is needed.
- Logical keys keep game code independent of storage locations and hashes; the closed catalog and limits keep what games may ship predictable for clients and storage.

## Alternatives Considered

### Keep signing per connection, now per asset

Rejected: one external signing call per asset per connection, and URLs that can expire mid-Session.

### A refresh endpoint for expiring URLs

Rejected by the human: a URL that always outlives its Session makes it unnecessary, and reconnections already get `session_started` again.

### Keep the batch in process memory

Rejected: each instance would sign its own batch, and a restart would sign again; the database shares one batch across instances.

## Consequences

- A signed URL works for up to 24 hours. It is a bearer credential to one immutable object of a game version: anyone holding it can download that object until it expires, and nothing can revoke it earlier.
- Signed URLs are stored in the database, readable by anyone with read access to `play_game_version_signed_urls`.
- A batch whose stored URLs do not cover every asset with an object is not reused, and the version is signed again.
- Seed data carries assets until the publishing flow exists.

## Canonical Knowledge Impact

- `play/docs/DATA_MODEL.md`: `play_game_version_assets` and `play_game_version_signed_urls`.
- `play/docs/FLOWS.md`: Frontend Access, and the required publishing rules.
- `play/CURRENT_STATE.md`: game versions, Start Session and seed data.
- `api/README.md`: `session_started.frontend.assets`.

## Implementation Impact

Implemented together with this decision: `play/session/usecases/frontendaccess` (reuse or sign, by Session), the asset catalog in `play/session/internal/business/gameversion`, `objectstore.Store.PresignGet` signing until a given expiry, migrations `20261026000000_play_game_version_assets` and `20261026100000_play_game_version_signed_urls`, the Runtime and API carrying the assets, and a new seeded Space Drift version with assets.
