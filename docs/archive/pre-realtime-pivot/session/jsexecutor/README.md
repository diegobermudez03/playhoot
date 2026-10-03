# jsexecutor

Status: PACKAGE-LOCAL OWNERSHIP NOTE

`jsexecutor` is a separately deployed workload — its own binary (`main.go` is `package main`), built and run independently of Session Runtime's own server process, communicating with it only over gRPC (see `proto/executor.proto`). See `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md` for why it is a separate deployment at all: it runs untrusted (including AI-generated) JavaScript and must not share Session Runtime's process, address space, database credentials, or secrets.

## Why this lives under `session/`

Being a separate *deployment* does not make this a shared or general-purpose service. `jsexecutor` exists solely to serve Session Runtime's own execution needs — per `ADR-0016`'s own text, it "is not a business bounded context," it is infrastructure, and per that same record, "Session Runtime still owns session execution semantics from the domain perspective; the Executor computes, it does not decide."

It is placed inside `session/` specifically to make that ownership visible: this is Session's own deployable unit, not a repository-wide capability sitting alongside `identity/`, `composer/`, or `orchestrator/`. No other domain may call it, depend on its `.proto` contract, or treat it as a general-purpose execution service. If a future need for shared/general-purpose script execution ever arises outside Session Runtime, that is a new, separate architecture question — it does not retroactively make this package shared.

Unlike `session/internal/...`, this package is **not** inside an `internal/` directory, so Go's own compiler does not block another domain from importing `proto/`. That restriction is intentionally not relied upon here (a separately deployed workload's own generated client stubs are conventionally kept import-visible for tooling reasons); this document is the actual boundary. Treat an import of anything under `session/jsexecutor/` from outside `session/` the same as a violation of an `internal/` boundary would be — because it is one, just not a compiler-enforced one.

## What it is not

- Not a business domain — see `ADR-0016`'s own "This is an infrastructure boundary, not a new business domain" section.
- Not a shared/general-purpose script-execution service other domains may call.
- Not part of Session Runtime's own process — it is a genuinely separate deployment Session Runtime's own code reaches only through a narrow `Executor` port (see `docs/projects/active/js-runtime-migration/works/WORK-0053-session-side-remote-executor-port-and-network-client.md`).

## Structure

- `main.go` — the service's own entrypoint (its own binary, not part of Session Runtime's `main.go`).
- `proto/` — the gRPC wire contract (`Executor.Execute`). Session Runtime's own future client (`WORK-0053`) is this package's only intended consumer.
- `internal/sandbox/` — the sandboxed JavaScript execution mechanics (QuickJS-on-`wazero`, process-isolated workers). See its own `LOGICAL_CONTRACT.md`.
- `internal/grpcserver/` — the gRPC handler wrapping `internal/sandbox`.
