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
  - locking lookup in memory sessions manager to get the session uuid's live manager
  - if it doesnt exist, then it registers it (it may have to fetch session data from session subpkg in this case)
  - then in that memory session manager, we check if there was already a live connection for that player (if request was duplicated), if there is, then we close that connection, we signal upstream to close WS connection
  - if it was new player, we validate the players count, validate that playuer is able to join
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
- something important, I originally wrote this step as making a call to the session subpkg to record the player connected to the session, but I decided thats not needed, we dont care if a player clicked join but then decided not to proceed with the game, he was never an actual player, we only record in DB when the session actually starts, but since I originally wrote it as making that session call, there might be some text referencing that which I forgot to update
- player count validation is made at runtime layer, so when the session manager is created we must get the info of the players so that we store them inside the session manager, so we dont need to perform that session subpkgh call

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
  - Call -> `/play/session` start session - input players connected, message originator user uuid and session uuid (runtime sessionn manager has that data)
    - Opens tx
    - Fetches session from DB
    - validates that session can be started (number of players, state, expiration, etc)
    - checks if user uuid is same from session creator
    - marks session as started
    - records all the connected players to the session
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
- players are recorded as joined only when the session actually starts

# Player disconnection

### input

- nothing, is live connection, BE already has the data linked

### other details

- This one is the durable player disconnection, as mentioned earlier the short network disconnection should be recovered internally by either the transport or the runtime layer, it has no effect on the overall session, this one is the actual durable one, which can happen when the player purposely disconnects, so the FE ws connection will send an actual disconnection message or when the network disconnection takes more than the given 3 seconds for the reconnection
- for the more than 3 seconds disconnection I dont know yet where the event would be triggered, as I dont know if we should keep that reconnection recovery at transport layer or runtime layer, if it was transport `/api`, then the disconnection message is sent by the `/api` layer to the `/runtime` one, but if its the runtime the one that handles it then is an internal event, for the sake of the flow I'll make it as if the message was always received from outside (which would be the case only for purposely player disconnection where the FE ws connection will send the actual disconnection message, meaning, it wont wait for the 3 seconds delay)
- naturally idempotent, if already disconnected then nothing needs to happen I mean, it can happen that we receive multiple times the disconnect message, and only the last one closes the WS connection

### flow

- > `/api` `live WS message with disconnection event`
- sends message to `/play/runtime` manager using communication mechanism yet to be defined
  - resolves the user uuid for that connection and the session uuid
  - if session is in LOBBY state, then simply removes the player fromm the session manager, no running session so no need to call script
  - if session is already running, then we call (or send message, yet to be defined how they'll communicate) `/play/runtime/sandbox` for that session context with the player disconnection event
    - executes disconnection event in the script
    - `RETURNS` the delta changes (yet to be defined actually but we're thinking on replicating state in `/play/runtime` and receiving deltas from these calls, using version number to validate we are sync)
  - `RECEIVES` the delta with the new state and user projections
  - (resulting state might be session terminated, in that case we'd perform whats described in the session t ermination flow)
  - replicates local session manager state with delta and submits new delta states to each player (using internal communication transport-runtime strategy yet to be defined)
- sends WS messages to players with deltas of player states

# Player rejoin

### input

- authorization tokens (required, even if its guest user, if its reconnecting then there must be at least a guest user associated)
- session uuid

### other details

- this one is the rejoin when the player actually disconnected but wants to reconnect, this is not the 3 seconds delay for network recovery
- eligbility must be set explicitely by the game, game must define if players can reconnect and whats the time window for it, even with that, script can perform internal logic to decide if allowing it or not
- when user joins the first time they get the session uuid, thats the session uuid that is passed to this request
- I was also thinking to allow rejoin by using same join code and join link, so that we'd receive the join code but since we know is a rejoin of a player we use it for resolving session, Actually now I'm liking more this option, or we can keep it with both options, session uuid used when the FE itself shows the rejoin option, FE has the session uuid srtored, and the join code for when the user wants to manually rejoin, even through a different device, but for the sake of the flow I'll only use the session uuid example

### flow

- > `/api`: Receives Restful Session re-join request
- Calls -> `/identity` resolve user uuid - PASSES user tokens and context of session join operation (if needed for permission validation, but I dont think we'll have)
  - operation requires user to hav an user uuid, so it must resolve to something, even guest user uuid
- `RECEIVES` user uuid
- Calls -> `/play/runtime` rejoin - PASSES user uuid and session uuid
  - locking lookup in memory sessions manager to get the session uuid's live manager
  - then in that memory session manager, we check if the player was still connected, if so, maybe the user is trying to switch devices, in that case, we remove previous connection, close its WS and continue with this rejoin request
  - communicate with `/play/runtime/sandbox` with the rejoin event for that player uuid
    - execute rejoin event with script (if the user was switching devices, then the player wasnt even considered as disconnected yet, so n oneed to perform logic, it will always be allowed, but if user was already marked as disconnected, then the script needs to perform its own logic)
    - Hand back the new detla state and projections and if reconnection was allowed
  - `RECEIVES` new delta state and projections and if connectionn was allowed
  - if reconnected, then it registers new connection, hands back full user projection (because user needs to render from scratch) and submits messages with new per player projections (event player reconnected) to the other players
  - `RETURNS` the channel/callback mechanism for transport-ruintime communication for the reconnected player
- `RECEIVES` if reconnected and mechanism for communication
- if reconnectd, then upgrades the connection to ws
- specifci mechanism to connect that live WS connection with the `/play/runtime` returned stuff

### other details

- actually, after writing the flow, I think I've decided to divide this operation in 2, one is switching devices, that should require no script interaction, it will always be allowed, the second one is the actual rejoin, where the user was already marked as disconnecteed, right now this flow mixes both, but imagine them separated

# Admin rejoin

I dont want to invest time in writing this one, because this is just that, the admin opening a new ws connection after disconnecting, but as the admin role, and for the admin role it has no impact on the playing session, it just will get the state and the projections so that admin can see with the view of the player it wants, no check apart fromm confirming admin is really the creator, thats all.

Is a Restful endpoint with the session uuid of the session or even join code also, we validate with the authorization tokens that the user is the creator, and then we upgrade connection to WS, thats all

# checkpoint

Right now cant differentiate this one from the replay one

# sample

# command

# impulse

# internal triggered events

Like timers, BE script can ask to controller to setup a timer, then controller simply submits the event when the timer is up, the BE script gets the event, and performs whatever action, for instance, if timer was for "if user hasnt responded in 5 seconds then we'll do something", the BE script simply outputs a timer of 5 seconds and the new state which asks that player for the response, controller knows nothing about the correlation, it simply hands the state back to the player and sets up the timer, then if player actuaslly responded back before the 5 seconds, the respopnse command would have already been processed by the script and therefore by the current state, controller will still submit the timer up event, but script should know to skip it as its no longer needed, there's no knowledge of the controller to cancel that timer because of that response

# Get current full state for player (for reconnection or reconciliation)

# Session recovery (internal pod error or stuff)

# Session termination

Can be terminated with

- Cancelled
- finished

# RECONCILIATION CRONJOBS

- sessions hanging in running with no running manager, it marks them as unexpectedly terminated
- expired sessions, marked as that, expired, never started

# other things to have in mind

- rate limits (for both input and output messages)
- command abuse to call command for what should be tick or impulse
- restrict session durations
- security, sandbox shouldnt have access to any machine or network resource, FE script shouldnt be allowed to perform any external operation
- assets handling, short lived signed URLs, FE script refers to assets as the key associated with them in the BE db, BE resolves short lived signed URLs and hands them back
- Space management, animations, FE-BE coordination, we might provide bultin functions or libraries to scripts, but that doesnt affect the overall platform design, so we'll think about that later
- how to monitor? granularity of the monitoring? yet to be decided, I have 0 experience with monitoring and observability with these live communication systems, I only know about observability monitoring and tracing for rest requests
- the session manager itself should have expiration, same as Sandbox context has expiration and session manager can renovate the sandbox context, we should contemplate having an expiration for session managers, they are memory resource consuming as well as go routine consumers, so we cant have infinitely opened, we must define like "closes after 1 min without interaction", that closes also the WS connections, but players might keep seeing simply last Screen state, if they move or do something after, then FE knows the WS was already closed so it calls a rest endpoint for like "reviving session", if session hasnt been marked as cancelled, then it can revive, setup a new session manager, and as the othr players interact once again they will connect to the new session manager
- el FE no decide arbitrariamente que tipo de mensaje es cada cosa, el script debe muy claramente especificar los tipos de interaccion, y que es cada uno, por ejemplo, el script puede definit que `move: SAMPLE` `click:IMPULSE` `buy_item:COMMAND`
- I was thinking, if the session manager sees at the same time hat it has 1 tick enqueued as well as a command and a sample, then it can send all of them in a single message to the sandbox, the sandbox operates them in order, and returns a single delta with the latest resulting state
- script DOESNT define tick frequency, buffer limits, etc, those are IMPOSED by our runtime platform
- FE can execute predictive logic, but it reconciles with the BE resulting state, BE is the source of truth, authoritative
- BE script handles stuff that affect the actual session, thats defined as logic that affcts the session and therefore other players projections basically, so for instabnce, taking the space movement example, the BE needs to know the distribution of objects that are important for decision, if x=4,y=10 has an item, and there's a logic that a player in that position grabs the item, then the BE scipt needs to know about that position and space movement, so it knows when to consider that a player grabbed the item, there's where the libraries or functions that help scripts easily sync FE and BE spaces are useful, but FE decorative stuff doesnt need to be known by BE script
- only predictive FE is predictive with it own player, like, if I'm playing and I'm pressing to mvoe to the right, I dont need to wait for BE new position state, I can move myself in the FE, and then reconcile with the BE, but for other players, I only see their positions as specified by the BE projection for me, so I might have a delay of 50-100ms on where I see them, but FE is smart, it wont print like first the other player in position x=2 and then all of the suddent after 100ms in x=5, its smart and it takes previous movement to continue with that speed, so it moves lets say 0.01 every ms, if then when we receive the new state position x=5 we only had it at x=4 iwth out prediction, then we also dont move in a ms 1 unit, we increas speed to rapidly reconcile, we must provde helper functions for this stuff as well for animations, that complex logic shouldnt be left to every single script, we can and must provide built in functions where script can link a state field with a FE item, and the function takes care of moving it and reconciling positions
- sandbox is sidecar, call between runtime and sandbox is internal machine call, not even network, relation is 1 to 1 with runtime and sandbox
- thinking, if sandbox dies but runtime service doesnt, should be kill entire pod and restart new one? should we try to start only new sandbox? but definitely if runtime dies and sandbox not, we need to kill everything
- as explained in flows, recovery is lazy, only if a FE player tries to reconnect we check and revive the session
- inputs are not persisted, only a vague state history for replay, but accuracy is not guaranteed
- recovery and replay are different stuff, how they are handle is different, shouldnt mix them
- we want to work based on deltas both from runtime sandbox communication, and FE BE communication, we use revisions/version numbers to veruify consistency, we provide in both communications reconciliation methods, so FE can ask for full state projection if something ha sbroken, same with BE and sandbox
- source of turht for other players is defined by BE and its projections, so if 1 player FE gets corrupted, it wont affect other players
- BE script should know absolutely nothing about FE details/events/decisions, it just processes events (messages), outputs new state (player projections) and FE script decides what to do with it, for instance, if we want to show a notification when something happens, BE shouldnt trigger "show notification", it simply shares through the state what happened, and the FE identifies what notification it wants to show given that what happened, obviously this cannot be enforced, because it depends on how the scripts are written, but this is the ideal rule
- we need restriction for all:
  SAMPLE
  - ingress max/user
  - latest wins

  IMPULSE
  - max/tick
  - max buffer

  COMMAND
  - token bucket
  - max outstanding
  - bounded queue

  GENERAL
  - websocket msgs/s
  - bytes/s
  - payload max

  EXECUTOR
  - CPU budget
  - memory budget
  - execution deadline

- games HAVE no external world interaction, scripts cannot access external world, and runtime platform shouldnt support any external world interaction asked by the script, this is for future, like, script shouldnt be able to perform payment requests, grant external permissions, etc, because for starters the session strategy is to have checkpoints but accpet data loss in case of failure, and simply continue from last checkpoint, if we were to perform exteernal action like charging a payment, that payment might be loss even though charged
- Scripts are inmutable once published in a version, a game version can only be played (session created) once its state is PUBLISHED, and once its state switches to PUBLISHED, the scripts are inmutable, and there's no such thing as deleting a game version, the game might get deleted, but the data of the game version still exists, unplayable though as the first filter is checking the actual game domain game state
