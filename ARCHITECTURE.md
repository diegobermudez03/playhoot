# Game pkg

This pkg contains all the implementation for running a session of a game, its divded in

## language pkg

This pkg contains all the definitions of the game definition languages, it just provides behavior for defining the game and compiling it, it doesnt care about actually running a session, it contains packages with versions such as `v1` `v2`, each version pkg contains the definition language and its runtime engines:

- We can have multiple definition languages, thats why its important to differentiate them with versions, and each definition language has its engines

Each version pkg contains:

### program pkg

Each version pkg contains a program pkg which exposes the program definition, not compilation.

### engine pkg

This is the actual engine for the defined language in `program` pkg. This pkg will always depend directly on the `program`'s types thats why they're defined in same pkg, so its clear that they need to exist together.

## session pkg

This pkg is a layer above language pkg, it handles the session, meaning, it executes a program (compiled game), and it takes care of the state persistence, so this layer is the stateful layer for the game session, look at the pkg's `README.md` for more information.

However, this pkg doesnt handle transportation protocol, it just receives method calls with the reference to the session, there must be a layer above which handles communication layer.

# General

- domains CAN NOT perform any type of external call:
  - Meaning, domains can only call internal services/repos/use cases, etc. those calls can be DB transactional
  - But for separate domains, we consider each domain as a separated service, so no DB transaction
  - But we dont want to have a complex graph where each domain performs external calls, handles consistency, compensation, etc.
  - So, instead, each domain can only expose methods that operate on its internal domain
  - packages `composer` `orchestrator` handle the cross service operations/workflows:
    - `composer` is stateless and handles reads that compose data from different domains
    - `orchestrator` handles write operations (workflows), either if approach is coreography or orchestration, its stateful as it needs to store request for the SAGA's

- `api` pkg is the only user facing expose pkg, exposes the endpoints, and internally redirects to appropiate domain (or `composer`/`orchestrator`)
