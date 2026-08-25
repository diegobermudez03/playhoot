This pkg is like the `gateway` for the entire system, this will be the only pkg exposing user facing endpoints:

- Exposes user facing endpoints
- Handles user authorization (JWT, etc)
- Can perform BFF (Backend for frontend) operations, exposing dedicated endpoints for FE specific purposes
- It contains NO STATE, it has NO DATABASE, it simply redirects calls to respective domains and can compose information for BFF
- While the system is a monolith the calls will be direct in program method calls, once we start moving services away the communication will be through internal network communication with rest API or gRPC
