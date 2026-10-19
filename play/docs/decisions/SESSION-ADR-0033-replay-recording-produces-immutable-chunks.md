# SESSION-ADR-0033: Replay Recording Produces Immutable, Self-Contained Chunks

Status: ACCEPTED
Created: 2026-10-18
Last status change: 2026-10-18
Supersedes: None
Superseded by: None

## Context

The architecture plan (§22) asks the Session Runtime to produce a durable, approximate history of every running game, so a future Replay system can show any participant's perspective of a past Session. Playback itself (reader, seek, API, UI, perspective selection) is out of scope. The plan describes Replay only in concept: in-memory chunks, a full state followed by deltas, transient records with their targets, independent cadences, bounded memory, and a durable final state for FINISHED games.

The design was settled by the human in a conversational session and handed to the Codebase Agent, which adapted it to the existing Runtime. One conflict with recovery appeared: a recovered game resumes at its last checkpoint, up to 5 s in the past, so it records game time that already-stored chunks may cover. That includes a rolled-back future that never happened.

## Decision

1. **Replay is not Recovery and not event sourcing.** Replay stores the authoritative results a game produced: sampled global state with its effective Session time, logical events, logical errors and COMMAND responses. It never stores inputs to re-execute, PRNG state, tokens, timers, connections, UserUUIDs or SAMPLE sequences. A future reader rebuilds a stored state and runs the pinned version's `project(state, participant, {sessionTime})` in the backend. The global state never reaches a client.
2. **Three independent knobs.** Game ticks (about 100 ms), state sampling and chunk duration never depend on each other. `REPLAY_STATE_SAMPLE_EVERY_N_TICKS` (default 1) keeps the state of one committed tick in N. Ticks are committed boundaries, not network messages. `REPLAY_CHUNK_DURATION` (default `60s`) sets the effective Session time a chunk covers, aligned to multiples of it. Transient records are always kept, whether or not their tick's state was sampled.
3. **Chunks are self-contained.** Each chunk starts with a full state (`base_state`). The states sampled after it are RFC 6902 JSON Patches from the previous state the chunk kept, not from the previous game tick. Then come events, errors and responses, all in timeline order (`timeline_order`, which also breaks ties of equal Session time). A chunk never depends on another. Every record that is not a state delta keeps who may see it. A COMMAND response or logical error always has exactly one participant. An event has a list of participants, or `null` for everyone.
4. **Versioned, compressed format behind a codec.** A chunk is JSON with a `format_version` (1), compressed with zstd (`github.com/klauspost/compress`), encoded and decoded only by the recorder's internal `chunk` package (`play/runtime/workflows/replayrecording/internal/chunk`). It is not a hand-made binary format.
5. **One recording per controller run.** Each controller that runs a game opens its own recording (`recording_uuid`). Its first point is the state it started or recovered from. Objects live at `replays/{sessionUUID}/{recordingUUID}/{index:06d}.rpl`, so recovery never rewrites an immutable object. When recordings overlap, the later one wins from its first chunk's start time. What was rolled back no longer exists, as in gameplay.
6. **Recorder in the Runtime, persistence in the durable Session.** `play/runtime/workflows/replayrecording` builds chunks in memory, one goroutine per recording. The recorder owns the chunking policy (sampling, chunk duration, size limit); its internal `chunk` package only offers chunk construction and the codec. Sealed chunks go to one bounded queue per instance (`REPLAY_MAX_PENDING_CHUNKS`, default 64), whose workers store them in the background with retries. Capture never waits for storage. When the queue is full, the oldest chunk is dropped and alerted. Storing goes through `play/session/usecases/storereplaychunk`: first the object is written to the existing private bucket (`replays/` prefix), then it is registered in the manifest (`play_replays`, `play_replay_chunks`). An orphan object is acceptable. A manifest entry without its object is not.
7. **FINISHED is durable, everything else best effort.** The controller hands the finished game's final tick to `Finish`. That tick always closes the last chunk, its transients and state included, even when it runs past the chunk's nominal end. `Finish` stores the chunk synchronously and finalizes the manifest before the Session becomes FINISHED. A failure is retried on the next tick. Other endings (cancelled, expired, aborted, shutdown) seal and store what the recording holds, best effort. A process that dies loses what was only in memory.

## Rationale

- Immutable chunks of bounded game time give incremental persistence, bounded memory for a future reader, simple seeking and limited loss when a pod dies.
- Deltas from the last kept state make sampling free to change without changing the format. JSON Patch handles removed fields, nulls and lists exactly, which a merge patch cannot.
- Per-recording keys turn recovery's rollback into an ordering rule instead of an overwrite or a forked history inside one object.
- Keeping persistence in `play/session` preserves the rule that only the durable Session writes durable state, and reuses its object storage.

## Alternatives Considered

### One global chunk index per Session

Simpler keys, but a recovered recording would collide with stored chunks. Skipping to the next free index would leave a rolled-back future visible in the replay. Rejected by the human.

### The Runtime writing replay tables and objects itself

Fewer hops, but it breaks the boundary that durable writes go through `/play/session`. Rejected by the human.

### gzip from the standard library

No new dependency, but less compact and slower on repetitive game states. The human chose zstd. The codec is versioned, so this can change.

### A separate replay bucket

Allows different retention and permissions. Deferred: the same bucket with a `replays/` prefix needs no new configuration per environment.

## Consequences

- A reader must apply the overlap rule (decision 5) and check `format_version` before decoding.
- The final chunk can be a whole chunk long, so finishing writes up to one chunk synchronously.
- Seed (test) data has no replay rows: a seeded manifest entry would point at an object that does not exist.
- There is no retention or deletion policy for replay objects yet.

## Canonical Knowledge Impact

- `play/CURRENT_STATE.md`, `play/docs/DATA_MODEL.md`, `play/docs/FLOWS.md`, `play/README.md`.
- `docs/projects/active/session-runtime-v1/CHECKLIST.md` section R.

## Implementation Impact

Implemented together with this record (2026-10-18). It replaced the replay placeholder.
