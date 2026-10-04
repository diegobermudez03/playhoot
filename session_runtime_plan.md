# Session creation

### input

- user tokens (authorization) (endpoint requries an account)
- game uuid

### other details

- endpoint is idempotent, receives idempotency token directly from FE

### flow

- > `/api`: Receives Restful Session creation request
- Calls -> `/identity` resolve user uuid - PASSES user tokens and context of create session operation (if needed for permission validation)
  - If the user is authenticated (logged in) then simply `RETURN` user uuid
  - If user not authenticated, return error, for this session creation authentication is mandatory
- `RECEIVES` user uuid (or if any error then error and terminates operation)
- Calls -> `/orchestrator` create session - PASSES user uuid and game uuid
  - Calls -> `/game` validate game is playable - PASSES user uuid and game uuid
    - Checks game visibility config
    - `RETURNS` if visibility is playable for that user (visibility could be private, where only the game creator can create a session, thats why we receive user uuid)
  - `RECEIVES` playable or not
  - if not playable returns that error
  - if playable, then proceeds
  - Calls -> `/play/session` create session - PASSES user uuid and game uuid
    - opens TX
    - Resolves game from DB
    - Checks there's a valid version with scripts for that game
    - eventually when we have restrictions, like at much X sessions per user in Y time frame, then we'd check here, only against session internal DB schema, which needs to keep track of player and their created sessions
    - creates the DB session pointing to the current game version, which will be the one used for the entire rest of the session, session is created in some kindof "pending" status
    - Creates join code for the session with expiration X time since right now
    - commits
    - `RETURNS` session uuid and join code
  - `RECEIVES` session uuid and join code
  - `RETURNS` session uuid and join code
- `RECEIVES` session uuid and join code
- `RESPONDS` with session uuid and join code

### notes

- Both `/game` and `/play/session` keep track of the same game uuid, each domain owns dirfferent part of the same entity (division by behavior, not by entity), `/game` owns the searchable/description/review/etc entity, whereas `/play/session` owns the executable part of it, versioning, etc.

### post-operations

- user is prompted to select if they want to join as host or as player

# Session admin join

### input

- user tokens (authorization) (endpoint requries an account)
- session uuid

### other details

- endpoint is naturally idempotent, if user was already registered as admin then no change, but `/play/runtime` and `/api` would add that new live connection, the same host can open multiple host connections, we can allow that, if user wants to connect from multiple devices, as long as its same account

### flow

- `WE NEED CHECK IF THE SESSION IS ALREADY BEING MANAGED IN A POD, IF THATS THE CASE, WE REDIRECT REQUEST TO THAT POD, NOT SURE WHERE WOULD THIS HAPPEN, IF BEFORE EVEN /API AT A PURELY INFRAESTRUCTURE GATEWAY LEVEL, OR IF INTERNALLY /API, ETC, SO OPEN QUESTION?`
- > `/api`: Receives Restful Session admin join request
- Calls -> `/identity` resolve user uuid - PASSES user tokens and context of create session operation (if needed for permission validation)
  - If the user is authenticated (logged in) then simply `RETURN` user uuid
  - If user not authenticated, return error, for this session creation authentication is mandatory
- `RECEIVES` user uuid (or if any error then error and terminates operation)
- Calls -> `/play/runtime` join - PASSES user uuid and session uuid
  - Calls -> `/play/session` connect user as host only - PASSES user uuid and session uuid
    - Opens tx
    - Fetches session from DB
    - checks if user uuid is same from session creator
    - checks if session is in a valid state and not yet expired
    - registers user as only host, not player in DB
    - commits tx
    - `RETURNS` bool user registered as host only or if any error
  - `RECEIVES` bool if i twas registered or error
  - if user was not registered as host, then `RETURNS` error
  - locking lookup inn memory sessions manager to get the session uuid's live manager
  - if it doesnt exist, then it registers it
  - then in that memory session manager, the user is registered as a live connection associated with that user as a host
  - `RETURNS` success or not, and if success, then returns the channel/callback or whatever specific approach so that upstream layer can connect the actual live WS connection with that manager
- `RECEIVES` success or not and the live connection mechanism stuff
- if not success `RETURNS` the error
- if success, then upgrades connection to WS
- specifci mechanism to connect that live WS connection with the `/play/runtime` returned stuff

### open thoughts

This operation creates the live session manager if it hasnt been created, how do we protect against concurrency? not creating managers in 2 pods at the time, imagine if 2 players are joining at the same time, my thoughts are:

- Since the entry point, there's a per session uuid locking query (the lookup for the pod for that session), if no pod registered for that session yet, then it inmediatly registers that pod as the owner, then it frees the lock on the session uuid
- but it can still happen that 2 requests are redirected to the same pod while there's still no memory manager for that session, then internally in the `/play/runtime` domain, when the operation succeeds and we're going to do the manager update, we'd lock the memory managers map (or however they are tracked) that locking lookup creates the manager if it doesnt exist for that session uuid yet, so the first request that arrives here will register the manager, the second one will simply find that one
- the issue here is that the pod was connected to that session in the first step, regardless of if the operation was actually successful or not, but we cannot revert that because in the meantime a valid operation might have been received and already redirected to that pod

# Session player join

### input

- user authorization (if exists)
