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
    - creates the DB session pointing to the current game version, which will be the one used for the entire rest of the session, session is created in some kindof "LOBBy" status
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
- This type of join doesnt allow creator to play but they can perform special operations with the live connection where they can switch views, viewing what each player is viewing, etc

### flow

- `WE NEED CHECK IF THE SESSION IS ALREADY BEING MANAGED IN A POD, IF THATS THE CASE, WE REDIRECT REQUEST TO THAT POD, NOT SURE WHERE WOULD THIS HAPPEN, IF BEFORE EVEN /API AT A PURELY INFRAESTRUCTURE GATEWAY LEVEL, OR IF INTERNALLY /API, ETC, SO OPEN QUESTION?`
- > `/api`: Receives Restful Session admin join request
- Calls -> `/identity` resolve user uuid - PASSES user tokens and context of admin join operation (but identity wont apply any permission based check here)
  - If the user is authenticated (logged in) then simply `RETURN` user uuid
  - If user not authenticated, return error, for this session creation authentication is mandatory
- `RECEIVES` user uuid (or if any error then error and terminates operation)
- Calls -> `/play/runtime` join as admin- PASSES user uuid and session uuid
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

- user authorization tokens (if exists), user is not required to be authenticated
- join code

### other details

- endpoint is naturally idempotent, if user was already joined then no op, although from connection level we'd replace the live connection with the latest one, for players there should be only 1 open connection

### flow

`SAME REGARDING RESOLVING THE POD FOR SESSION UUID, ALTHOUGH HERE IS MORE  COMPLEX BECAUSE WE DONT EVEN HAVE THE SESSION UUID, ONLY THE JOIN CODE, NOT SURE HOW WE CAN HANDLE THIS`

- > `/api`: Receives Restful Session join request
- Calls -> `/identity` resolve user uuid - PASSES user tokens and context of session join operation (if needed for permission validation, but I dont think we'll have)
  - If the user is authenticated (logged in) then simply `RETURN` user uuid
  - If user is not authenticated but there are tokens linked to a guest user, then resolve the user uuid associated with that guest user and `RETURNS` user uuid
  - If user is not authenticated nor has a linked guest user, then a new guest user is created for user and tokens are `RETURNED` along with the generated user uuid, FE should store those tokens so that other operations are linked with that same guest user, for most domains they dont know the difference between actual account and guest user, they just treat with user uuid, is `/identity` the one that knows the difference and can resolve each specific account, also `identity` is the one that will perform request allowing/denial based on guest vs account permissions, other domains dont need to know, if user decides to upgrade the account to an actual account, we'll upgrade that guest acount, so that they keep all what they did with the guest account, we depend on the FE to reuse the tokens and not cleaning them up. For guest users we'll ask for the username, so if no username received then we might need to hand the request back to request the FE to ask for the username so that we can link the guest user
- `RECEIVES` user uuid
- Calls -> `/play/runtime` join - PASSES user uuid and join code
  - Calls -> `/play/session` join player - PASSES user uuid and join code
    - Opens tx
    - Resolves session by join code
    - validates eligibility (not expired code, session in "LOBBY" state, allowed number of players, etc)
    - joins user as player
    - commits tx
    - `RETURNS` bool user joined and resolved session uuid
  - `RECEIVES` bool if user was joined successfully and resolved session uuid
  - locking lookup in memory sessions manager to get the session uuid's live manager
  - if it doesnt exist, then it registers it
  - then in that memory session manager, we check if there was already a live connection for that player (if request was duplicated), if there is, then we close that connection, we signal upstream to close WS connection
  - then we register the current connection
  - `RETURNS` success or not, and if success, then returns the channel/callback or whatever specific approach so that upstream layer can connect the actual live WS connection with that manager
- `RECEIVES` success or not and the live connection mechanism stuff
- if not success `RETURNS` the error
- if success, then upgrades connection to WS
- specifci mechanism to connect that live WS connection with the `/play/runtime` returned stuff

- I list this operation here because it doesnt need to be sync with the main join operation, but the manager would internally send the pdate of the event to all the connected players, so that they see that a new player connected

### other details

- if creator decided to join as player then they'll hit this one instead of the session admin join
- this applies for all WS connections, we need to have a mechanism so that we are tolerant to brief disconnections, if WS is disconnected but then a new request is sent like 0.1-3 seconds after then we should be able to reconnect them without even reaching the game script, after those 3 seconds then we can start the disconnection process, but I dont know in which layer should this live
  - Should `/api` be the one handling this? so that if it disconnects, it simply waits those 3 seconds for another WS request, so that it can link that new WS request with the channels/callbacks for the player?
  - or should this be handled at `/play/runtime` layer, so `/api` inmediatly tells runtime about the disconnection but is runtime the one that waits before processing the disconnection?

# Start session message

### input

- nothing, is live connection, BE already has the data linked

### other details

- even if creator joined as player, they'll be the only ones that can start the session, they'll see the start button, FE knows that they must be shwown that button because frontend knows they are the creator, thats why they have session uuid
- We could return the info about the number of players expected for the game during the session creation, so that the FE can show up the button to start session based on if the required number of players is already connected, BE will anyways reject, but better to avoid false expectations in FE
- since this is a live connection message, there's no identity check, the live connection openning is the one that needs to be authenticated
- operation is kindof naturally idempotent, so no issue there, if the session call succeeds but subsequent script call fails its fine, a retry would simply do no op in session pkg (for marking as started, it will store the snapshot), sending the messages to players is the last step, so that the operation only has effect in users if all steps succeed, if script eecution fails because of some non transient error, we can have handling to inemdiatly mark session as cancelled because of that error and send messages to players (just as clarification, the descriptions in this file are not any actual detailed escriptionn which consider all cases, no, its just an overview of the general idea, not the detailed way it will be implemented, thats why I write this points here and they dont show in the flow section)

### flow

- > `/api` `live WS message with start event`
- sends message to `/play/runtime` manager using communication mechanism yet to be defined
  - Call -> `/play/session` start session - user uuid and session uuid (runtime sessionn manager has that data)
    - Opens tx
    - Fetches session from DB
    - validates that session can be started (number of connected players, state, expiration, etc)
    - checks if user uuid is same from session creator
    - marks session as started
    - fetches scripts and stuff
    - commits tx
    - `RETURNS` scripts or validation error
  - `RECEIVES` scripts or validation error
  - External call to -> `/play/runtime/sandbox` create session context - INPUT BE script and other required details for context
    - create context for that session (with some expiration time) (specific underlying handling still not yet defined, it could be an actual go routine per context, or simply memory contexts but single goroutine, etc)
    - Loads script, initializes, anything needed (I lack coontext of what is done here)
    - executes script with start signal and list of players to get initial global state and player individual projections
    - `RETURNS` initial global and per player projection states
  - `RECEIVES` initial global and per player projection states
  - Call -> `/play/session` to store initial state snapshot - input global state
    - Not yet defined how we'll handle this state replay storage mechanism and the checkout stuff, it might be 2 sepaate tables or storage mechanism, it doesnt have to be relational database persistence, because its 2 separate purposes, one is to store checkpoint snapshots for session recovery, the other one is purely state recovery after an outage or something, so this step is yet to be defined, but we know that there will be this call which internally stores both checkpoint state snapshots and replay snapshots
    - `RETURNS` error or nothing
  - Stores in memory the global state
  - `Submits messges` to all connected users signaling the session start and it contains the per player projection state, also sends the FE script for them to render
- specifci mechanism to connect that live WS connection with the `/play/runtime` returned stuff

### other details

- this one applies for all ws connection messages, they go directly to a specific session manager, and session manager will serialize their processing, but anywways when we do DB oeperations we should also lock session just in case
- I was thinking, the sandbox context is allowed to be expired, every interaction between the session manager and the sandbox should be prepared to get a "no context" error so that it re-initializes and creates a new context
- I was thinking, we shouldnt rely on the BE script to handle the per user projection, that should be enforced somehow, even maybe as a separate script, having 1 script which performs global state computation, and a separate one which only receives the global state and returns the per player state, the reason is for replay but also correctness, but for replay, we"ll simply have some reconstructed history of the global state, not the per user state, so we have to use the projection script to generate the per user state which then is sent to the replay FE which is simply the same FE script

# Player disconnection

# Player rejoin

# Admin rejoin

# Get current full state for player (for reconnection or reconciliation)

# Session recovery (internal pod error or stuff)
