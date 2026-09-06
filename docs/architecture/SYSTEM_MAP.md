# System Map

Status: CURRENT IMPLEMENTATION

```mermaid
flowchart TB
    Main["main.go\nRuntime entry point\nopens PostgreSQL and runs migrations"]
    Migrations["migrations.go\nRuns Game Management and Session Runtime migrations"]

    subgraph GameBC["Accepted Game bounded context"]
        GM["game/game\nGame Management capability\nimplemented model, storage, migrations, get-game use case"]
        SR["game/session\nSession Runtime capability\nstorage/migrations and session lifecycle scaffolding"]
        GL["game/language/v1\nGame Language supporting subsystem\nprogram definitions, codec, compiler, runtime engine"]
    end

    Logging["logging\ntechnical library"]
    Monitoring["monitoring\ntechnical library"]
    Utils["utils\ntechnical/test support"]

    subgraph DocumentedRoles["Accepted roles / implementation not started"]
        API["api\ntransport/application edge\nREADME only"]
        Composer["composer\ncross-domain read coordination\nREADME only"]
        Orchestrator["orchestrator\ncross-domain write coordination\nREADME only"]
    end

    Main --> Migrations
    Migrations --> GM
    Migrations --> SR
    GM --> GL
    SR --> GL
    GM --> Logging
    GM --> Monitoring
    GM --> Utils
```

## Repository Boundary Notes

- Some top-level directories represent unresolved/provisional boundaries.
- API, Composer, and Orchestrator currently exist as documented roles, not runtime implementations.
- See `../../ARCHITECTURE.md` for accepted architecture rules.
