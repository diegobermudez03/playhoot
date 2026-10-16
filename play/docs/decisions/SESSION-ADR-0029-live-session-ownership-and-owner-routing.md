# SESSION-ADR-0029: Live Session Ownership And Owner Routing

Status: ACCEPTED
Created: 2026-10-15
Last status change: 2026-10-15
Supersedes: None
Superseded by: None

## Context

The architecture plan (`plans/session_runtime/Playhoot_Session_Runtime_Architecture_Plan_with_TOC.docx`, §1 "Runtime pod topology") requires that an active Session has exactly one Runtime owner, that every WebSocket for a Session terminates on that owner, and that the browser never learns a pod address. It names an owner abstraction (`Resolve`, `Claim`, `Release`) and leaves leases, epochs and fencing to implementation.

Every live entry point (admin join, player join, rejoin, recovery reconnect) must first find the owner. Whichever request arrives first for a Session without an owner (a player's join or the creator's admin join) is the one that makes it owned, so finding the owner is always "resolve or claim", never only "resolve".

Today Playhoot is one binary (API edge and Runtime together) and runs as a single instance. The expected scaled topology is different: `/api` becomes its own deployment, a pure gateway, in front of a pool of Runtime pods.

## Decision

1. **Ownership and routing are separate concerns.**
   - *Ownership* (which Runtime instance owns a Session, under which epoch, until when) belongs to the Session Runtime (`play/runtime`). The Runtime is the one that must stop mutating when it is no longer the owner and that releases ownership when a Session ends.
   - *Routing* (getting a request to the owner) belongs to the API edge, which owns transports.

2. **The API edge sees one atomic operation: locate or claim.** It returns the current valid owner of the Session. If there is none, it atomically claims the Session for an instance and returns that instance. Locking the Session, checking for an existing owner and registering the new one are a single atomic operation (lock or compare-and-set), so two callers can never both claim. Only Sessions that exist in a connectable state are claimed. The result carries the owner's internal address (never sent to a browser) and an ownership epoch.

3. **Which instance gets a claimed Session is a placement policy inside the ownership implementation, not an API decision.** While the API and Runtime are one binary, the policy is "the instance handling the request". With a separate gateway, it becomes "any Runtime pod" (or the least loaded). The API edge compares the owner address with its own: when they match it handles the request locally, otherwise it proxies it. With a gateway they never match, and the gateway always proxies, for the whole life of the connection. That is the gateway's job and does not make Runtime pods relays for one another.

4. **The ownership epoch is the fencing token.** The Runtime checks it when a transport is attached and on every durable write that depends on ownership. A Runtime that is no longer the owner rejects the attach, and the client reconnects and is routed again.

5. **A heartbeat-renewed lease decides whether an owner is still valid. It is not implemented while ownership is in-process.** See "Instructions for multi-instance ownership" below.

## Instructions For Multi-Instance Ownership

The in-process implementation used while Playhoot runs as one instance has no heartbeat, lease or shared registry: the process that holds a Session is its owner until it releases it or dies, and dying loses everything anyway. That stops being true **as soon as more than one instance can own Sessions**. That happens when the gateway is split out, but also if the single binary is ever run with more than one instance. At that point, implement all of the following before allowing it:

- **Shared registry.** Ownership is stored where every instance (and the gateway) can see it: `sessionUUID -> owner address, epoch, lease expiry`.
- **Lease + heartbeat.** The owning Runtime pod, not the gateway, renews the lease for the Sessions it owns at a fixed platform-time interval. A Session whose lease has expired has no valid owner: the next locate-or-claim may assign a new owner, bumping the epoch. Lease and heartbeat durations are platform-time tuning values. The lease must comfortably exceed the heartbeat interval so one late heartbeat does not move a healthy Session.
- **Self-fencing.** A Runtime pod that cannot renew a lease (registry unreachable, lease lost, a newer epoch seen) must stop mutating that Session and close its transports. Otherwise two pods could mutate the same Session after a network partition, which violates the single-owner invariant (plan §30.1).
- **Epoch check on durable writes.** Writes that only the owner may make (Start's final transaction, checkpoint metadata, terminal transitions) carry the epoch and are rejected when it is stale.
- **Release.** The owner releases ownership when a Session reaches a terminal state and when its controller is destroyed (checklist A1).
- **Owner loss is observable.** An expired lease is how recovery (plan §21) and the reconciliation of RUNNING Sessions without a valid owner (plan §23) detect owner loss.
- **Gateway authentication.** Once the gateway is a separate deployment, it authenticates the caller once and passes the resolved userUUID to the Runtime over a trusted internal channel. Until then, authentication runs in the same process as the Runtime.
- **Infrastructure.** Proxying to a specific Runtime pod needs addressable instances. Cloud Run (the current target, see `main.go`) cannot address a specific instance, so the multi-instance topology also needs a deployment decision.

## Rationale

- Keeping ownership in the Runtime and routing in the edge matches who must act on each: only the Runtime can stop mutating when it is no longer the owner, and only the edge holds the transport.
- A single atomic locate-or-claim removes the race of a separate resolve followed by a claim.
- Putting placement behind the ownership implementation lets the API code stay the same from single-instance to gateway: only the placement policy and registry change.
- A proxying gateway is what keeps pod addresses away from the browser. Browsers also do not follow redirects on a WebSocket handshake.

## Alternatives Considered

### Redirect the client to the owner pod

Rejected: it exposes pod addresses to the browser (forbidden by the plan), and browsers do not follow HTTP redirects during a WebSocket handshake.

### Runtime pods proxy to each other (no gateway)

Acceptable only as a fallback while the API and Runtime are one binary. As the scaled design, it doubles hops between peers and makes a relaying pod's restart drop connections it does not own.

### Separate Resolve and Claim calls at the edge

Rejected: two edges can both see "no owner" and both claim. The claim must be atomic anyway, so it is exposed as one operation.

### Gateway-only ownership registration (no Runtime heartbeat)

Rejected: the gateway cannot tell when a Runtime pod dies, so it would keep routing to a dead owner.

## Consequences

- The API edge has one owner-routing step before every live WebSocket upgrade. While single-instance, it always resolves to the local instance, and the proxy path exists only as a placeholder.
- Multi-instance deployment is blocked on the instructions above. They must be implemented, not merely configured.
- The ownership epoch becomes part of the API-Runtime attach boundary.

## Canonical Knowledge Impact

- `play/README.md` - live Session ownership is owned by the Runtime, and routing to the owner by the API edge.

## Implementation Impact

Owner routing middleware in the API edge, an in-process ownership implementation in `play/runtime` (single-instance placement, no lease), and the epoch passed into the Runtime attach. Proxying to a remote owner is a placeholder until the multi-instance topology is decided.
