# Project: Game Creation v1

Status: ACTIVE
Created: 2026-11-01
Last updated: 2026-11-01

## Goal

Deliver the game creation flow of `../docs/04_Playhoot_Plan_del_Flujo_de_Creacion.docx` (the "plan"; `§N` points to its chapters), deliveries 0 to 3 (§13): a creator (with an external AI that only reads links, with an AI agent through API/MCP, or alone) edits a persisted draft, uploads its assets, builds it with the Playhoot SDK, tests it in a human preview and in automated scenarios, and publishes a validated build that other people can play. The SDK contract is `../docs/03_Playhoot_Plan_del_SDK.docx`.

The frontend counterpart is the frontend repository's Project `game-creation-web-v1` (SDK packages, build tools, authoring UI). As in `session-runtime-v1`, the human directs each step in conversation; `CHECKLIST.md` tracks what is required and done.

## Explicitly Out Of Scope

- Plan deliveries 4-7 (cards/boards, didactic simulation, 2D world, launch closure): later projects.
- Playhoot's own AI, billing, automatic screenshots, autonomous template search (plan §1).
- Infrastructure and real integrations (deployed storage, isolated build worker, global quotas, real MCP clients): done before the first deploy, tracked in `docs/engineering/LAUNCH_READINESS.md`.

## Decisions Taken In Conversation (2026-11-01)

- Assets use the final flow from the start (reserve, signed upload URL, finalize with server-side validation) behind a storage provider; a real local provider (disk + signed URLs served by the backend) is used until the GCS one at launch. No code is written that integration would have to replace.
- MCP is implemented for real in this binary. Its authorization is per user account (OAuth); the project travels in each call. Identity reuses the normal sign-in but keeps AI authorization attempts and AI sessions in their own tables.
- One MCP operation = one API endpoint = one creation service method. Services know nothing about actors: authorization lives in the upper layers (API, MCP, Identity), sharing one authorization component. MCP calls the service method directly; if split out later it calls the endpoint.
- The project link given to an AI grants read and write; an AI that cannot write tells the user what to paste. A GET never mutates.
- Real AI clients are tested by the human on a throwaway GCP VM with an ephemeral database; no tunnels.
- Backend scripts built by the SDK use a new versioned scriptcheck profile `sdk-v1` (CHECKLIST B): top-level module state is forbidden statically and top-level constants are frozen once at load; the author may mutate the state it receives (each call gets freshly parsed JSON). No per-call re-initialization. Code that still mutates something top-level breaks the protocol and is a bug of that game, accepted.
- The human preview goes before automated scenarios.

## Completion Criteria

Every CHECKLIST item done or explicitly moved out by the human, and the plan's delivery 3 closure (§13) demonstrated: the same kind of game created by a read-only chat and by an agent with tools, both built and tested, published by the owner and played by another person.
