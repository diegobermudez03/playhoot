# Object Storage Standard

Status: CANONICAL ENGINEERING STANDARD

Accepted by the human on 2026-11-02, while designing DRAFT asset uploads (`docs/projects/active/game-creation-v1`).

This standard applies to every flow that moves bytes between a client, the backend and object storage, and to every write that stores an object and references it from the database.

## When A Signed URL Is Used

1. A signed URL is issued only when, in that flow, the bytes never need to pass through the backend: players downloading a version's frontend and assets, or previewing an asset the backend already validated. It saves the backend the payload's compute and network.
2. When the backend must read or validate the bytes anyway (an upload whose size, real type or hash it checks; content it transforms), the bytes pass through the backend and no signed URL is issued. A signed URL there only adds transfers (client → storage → backend → storage) and lets oversized content cross the network before it can be refused.

## Receiving An Upload

3. The body is read as a stream with the limit of what it declares to be (for example the asset type's maximum), plus one byte to tell an exact fit from an overflow. Past the limit it is cut at once and answered `413`, with nothing stored.
4. The content is hashed while it is read, and its real type is decided from its bytes, never from a name or a header.
5. It is stored only after every check passed.
6. Each instance bounds how many uploads it holds in memory at once; a request beyond the bound waits briefly and is then refused as retryable (`503` with `Retry-After`).

## Writing An Object And Its Reference

There is no transaction across storage and the database. The order makes every failure harmless:

7. Bytes first, reference second, never the reverse. A database row pointing to an object that does not exist must be impossible; an object nothing references is harmless.
8. Keys are content-addressed (`<prefix>/<sha256>`), so a retry rewrites the same object and goes on.
9. Before storing, an intent row registers the object, idempotent by its hash, so every object that may exist is known. A failure after it leaves at most a registered object nothing references; the owner's cleanup deletes registered objects no one uses (deleting one that was never stored does nothing).
10. The reference to the object is written only after the store succeeded, in the transaction that decides its use.
