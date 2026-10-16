# System Map

Status: CURRENT IMPLEMENTATION

```mermaid
flowchart TB
    Main["main.go\nRuntime entry point\nopens PostgreSQL, runs migrations (and test data outside production),\nserves the API, graceful shutdown"]
    Migrations["migrations.go\nRuns Game Management and Session Runtime migrations and seeds"]
    API["api\ntransport/application edge\nPOST /sessions, auth middleware, request metrics"]

    Identity["identity\nIdentity domain\nauthorize use case (placeholder: fixed test tokens)"]
    GM["game\nGame Management domain\nmodel, storage, migrations, check-visibility,\ncheck-playability (placeholder: always playable)"]
    SR["play\nSession Runtime domain\nSession creation (sessionlifecycle), play_* schema,\nsandbox helper; live runtime not implemented"]
    Orchestrator["orchestrator\ncross-domain write coordination\nplaylifecycle: CreateSession"]

    Logging["logging\ntechnical library"]
    Monitoring["monitoring\ntechnical library\nalerts, metrics entry points"]
    Utils["utils\ntechnical/test support"]

    subgraph DocumentedRoles["Accepted roles / no workflows yet"]
        Composer["composer\ncross-domain read coordination\nREADME only"]
    end

    Main --> Migrations
    Main --> API
    Migrations --> GM
    Migrations --> SR
    API --> Identity
    API --> Orchestrator
    Orchestrator --> GM
    Orchestrator --> SR
    API --> Logging
    API --> Monitoring
    SR --> Logging
    SR --> Monitoring
    SR --> Utils
    GM --> Logging
    GM --> Monitoring
    GM --> Utils
```

Game Management and Session Runtime are independent top-level business domains, not capabilities nested under a shared "Game" bounded context (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`). Session Runtime is being redesigned as a real-time runtime (`play/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`); its current contents are described in `play/CURRENT_STATE.md`. The only implemented cross-domain flow is described in `CROSS_DOMAIN_FLOWS.md`.

## Repository Boundary Notes

- Some top-level directories represent unresolved/provisional boundaries.
- Composer exists as a documented role with no implemented workflows.
- See `../../ARCHITECTURE.md` for accepted architecture rules.
