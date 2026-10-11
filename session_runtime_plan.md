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

I reallly dont know exactly how this one would work, what I imagine is

- `/play/runtime` internally has a timer which triggers the checkpoint
- session manager takes a snapshot of the global state, and persists it in persistent storage
- I have no idea if the persistent storage should be relational DB or another type
- What I have in mind is to make these checkpoints best effort, when the checkpoint is triggered, it inmediatly marks it as completed and schedules next checkpoint, then it tries to persist the snapshot, if the persist attempt fails, doesnt matter, we wont retry, we'll wait to next checkpoint

# FE BE COMMUNICATION PROTOCOL

I'll try to document how I see the entire protocol working.

SO in my mind there will be 2 types of messages:

- Commands: These are pretty straightforward, follow the request-reply pattern, is a single message which requests something and receives an unique response (and other players side effects) for it, games such as cards games, board games would run enterily on top of commands, even though they use the same websocket connection for messaging, it is to build a request-reply pattern (discrete communication)
- Samples: These are for live communication, which can represent movement, real time continuous communication.

In both types is important to understand whats the result, the result is in general purposes:

- response data for the action user
- new state deltas per player, these are projections, script must implement fuinctions which receive the global game state, the per player projection, and returns the delta of the per player projection state
  - For every state change we have the concept of version number, same as we have between the `/play/runtime` and `/play/runtime/sandbox`, where runtime checks that it is in sync with sandbox by using version number, if not it reconciles, the same applies for projections between players and BE, player must follow the version number, the state interactions are built on top of deltas and manually calcualting new state, not on top of sending the full state always, so its important for components to make sure the deltas received have the expected version number, we can expose a verification endpoint so that clients can every a while confirm that the manually calculated state corresponds to the actual expected state for the given version number

## command

### input

- the request will have a specific body payload defined for that type of command, the input is just that payload

### flow

- > `/api` `live WS message with command event`
- sends message to `/play/runtime` manager using communication mechanism yet to be defined
  - write command into buffer of actions to be submitted

  - then async the reader of the buffer will eventually take the command
  - sends the command to `/play/runtime/sandbox` using the session context
    - sandbox executes the command
    - sandbox `RETURNS` the delta of the global and per player projections state
  - `RECIVES` the delta of the states and stuff
  - updates runtime mirror of the global state using deltas
  - Sends messages to players with new deltas of their states

## sample

Samples are way more complicated, i was thinking a lot of things:

- this is what make the games be device dependent, because the continuous input capability depend on where it comes from (keyboard, mouse, finger), but BE game shouldnt be aware of the FE component and way of manipulation.
- so I think that the game itself simply defines inputs, those are logical inputs, not specific physical interaction entry points, but logical ones, so instead of defining `key right` it would be `move right`.
  - then the BE isnt aware of what physycal stuff is needed or a given game, it just knows the operations that can be applied on it
  - it becomes FE script responsability to map those operations to their inputs, ofcourse we'll have some kindof thing to store that in the BE so that our BE is smart and every search or stuff endpoint is queried by current type of device and we show games allowed for that device, but that will be metadata at that point, the BE engine itself shouldnt care technically
  - so if FE script can map the inputs to every device, thenn game can be played for every device, is the FE script the one that knows about the interaction with user
    - but, the geenrated script is not the one that will connect with the keyuboard events, or mouse events, it;ll just interact with abstractions, recall that the script is dangerous if we give it access to the device with no constraints, so is our FE sandbox the one that provides built in abstractions to interact with touch, mouse, keyboard, etc, the script internally maps those abstractions into the operations
    - our sandbox will also provde the reader methods such as `isMobileDevice()`
    - so a game could for instance check if its mobile device, if it is, it can build the keys it needs as UI components, so that it allows to play the game even with no actual keyboaard, if its not mobile device then it uses directly the provided abstractions to interact with keyboard events, but again, is FE script the one that gives support to different devices, the game definition itself only defines type of samples, and its up to FE script to know how those samples are generated
- the samples should always represent what the client is doint, and BE calculatee whats the outcome of that (this is for samples, not commands), so what we receive in every sample is the current state of the inputs, nothig calculated, just the current value

So lets say a game defines that its inputs are (this is just pseudo stuff):

```
  movement_right -> bool
  movement_left -> bool
  movement_up -> bool
  movement_down -> bool

  selector_position -> (x,y)
```

where the movements inputs are mapped to keys or buttons, where value is only true if pressed at that given time and false if not
for those the FE doesnt tell "based on the movement pressed player position is X,Y", it simply says "at this moment the movmeent right was pressed", BE will calculate whats the position

But another type of operation may have a different type of input, in this case the selector, which could be the finger, or the mouse, that one is a position, but is not a logical internal game position, is literally the position of the mouse/finger at that tgiven time in relation to the game's screen area, is up to the BE to decide what to do with that position, if that position maps to some internal game position as the characteer position, then thats fine, but again, we're not technically allowing the FE to decide internal part of the game, its resonsability is just informing how the user is interacting with the game.

So, every X time (lets say 100ms) the client will send its current state of inputs, but additionally it can send always that it detects a change, so for instance, every time the key of move right is pressed, it can submit the sample, then when its stopped being pressed, it can send another sample, without waiting for the 100ms round, but its just for the changes, not like sending everytime the key is pressed, is sending everytime the curretn state of inputs has changed. but for stuff like mouse, which is compkletely continuous and can change evry ms we do have to not allow submit sample on every change but rate limit it.

(just a note, when I say that we send the delta, I dont mean that for instance, if the projection state had a field position in x, which before was 50 and we just calculated that now is 80, then we send 30, thats now that I mean, we'll be sending the new 80, what I mean by delta is that we;ll only be returning the fields that had a change in their values)

Now, from the BE, we'll be receiving the periodic input states, we'll mark each one with its received timestamp, that one will be used for velocity, speed, aceleration calculations. Its not perfect as the timestamp doesnt correspond to the actual time when the action happened at FE, but when it was received, so it has latency, but its the safest option, as we for sure dont want to trust the FE, we'll be pasing those durations to the script so that it can (if needed) calculate speed, velocity, etc, BUT, we wont be sending the actual timsetamp, script doesnt care when the game is being played, we'll be sending based on the time since the game was started.

So, the BE receives a periodic input from a client, what it does is that it marks it with the current timestamp, and then it enqueues it into the for processing updates queue, but this enqueueing is smart, it;ll check the previous state, and it;ll only cntain the fields that actually had a change, lets say that previous sample already had movement_right pressed down, then we will keep thatt one and wont be repeating it in the new sample, that field would only appear once we receive a sample where it has a new value, this is kindof of a dedup, because if we decide to process a tick for every 100ms, and we received 20 samples in those 100ms, we want to be smart, most probably from those 20 samples only 3 of them had changes, then we'll be sending only those 3 changing samples to the script.

But so then going back, when the sample is received, its just marked with the timestamp, its deduped based on the previously enqueued waiting for tick samples for that given user, and thats all.

then, the controller has the ticks worker, which every X time takes the enqueued buffer for a given player, and it passes that list of samples(which is already deduped) to the BE script, BE script must expose the entry point for samples processing, which receives the list of samples with the times (based on game initiation)

Because of the mechanism of how we treat samples we can use samples for both samples and impulses, because we could have stuff like:

```
  s1: movement_right: true
  s2: movement_right: false
  s3: movement_right: false
  s4: movement_right: false
  s5: movement_right: false
  s6: movement_right: true
  s7: movement_right: true
```

thjat will become and be processed as:

```
  s1 at t0: movement_right: true
  s2 at t2: movement_right: false
  s6 at t6: movement_right: true
```

so if logical behavior was continous movement, we;ll process it very accurate, as we will even take into account the internal changes within the ticks, but also, if logically it was impulses (like if game cares about number of times when the key was pressed) then we also have that information

Now its important to understand how the FE would work, we already know how it'll communicate with BE, what signals its going to send, and whats going to receive in exchange (the delta of the current player projection state), but then, whats the FE going to do? here we have to differentiate between how the player sees its own changes and how it sees other players changes:

- For own changes, the FE script can be predictive, if the rule is that when pressing right the movement will be at 5units per second, then FE can inmediatly show that change without waiting for BE to send the new state, the space constraints are shared between FE and BE scripts, we probably should eventually provide built in functions/libraries so that all that shared behavior is provided and dont need to be writen from scratch always, but what I mean is that, for intance, is like fortnite, BE must know that there's a wall at position x=5, y=10, so that when it calculate sthat the player is there it doesnt allow to move more, but the FE also knows about that constraint, so the FE when its predictive, before waiting for BE feedback, can also stop the movement when it detects that, ideally what the FE predicts should be very close to what BE actually says, the discrpeancy should only be because of the network latency (if movement started at ms=0 in FE, it may be taken for calculationn as started as ms=14 in BE), BE only needs to know about the space constraints that affect what happens in the game, the decisions, etc, but it doesnt need to know about FE stuff, how the game sees, decorations, animations, etc, for the BE its just a space determined by x and y axis, where there might be stuff in some places, for instance, if there's a circular object, BE dont neccesarily need to have a complex geometrical calculation in the fgemoetrical space for that circle, it could do it, but it could also simply mark the space where that circle is with small square areas, so it wont be 100% accurate, but close enough. But then the point is, FE sends signals, FE inmediatly renders, applies space constraints, then, whent he BE sends the response, the FE might reconcile if the difference is too much, or do nothing if its very close, but here there's something important, and this makes the versioning vbery important, suppose this

```
 FE sends s1 at t1
 FE sends s2 at t2
 FE sends s3 at t3
 FE sends s4 at t4
 FE sends s5 at t5
 BE takes s1-s5 to processing at t5
 FE sends s6 at t6
 FE sends s7 at t7
 BE finishes processing s1-s5, returns expected result state to player
 FE receives s1-s5 expected state, meaning, expected state at t5
 but FE was already rendering t7, so if FE goes back to t5 it would be ignoring the already predicted rendered changes for t6 and t7, so FE needs to understand that whats receeiving is the reconciliation for time t5, it should understand what changes it has applied since then, and then it can reconcile but smartly, so, it should know for instance

 FE at t5 thought pos_x=5.6
 FE at t7 has pos_x=6.7
 But BE reconciliation for t5 returned pos_x=5.3
 FE considers the gap to big, but it understands is a gap for t5, not t7, so it applies backwards changes to reconcile, basically, the difference in pos_x at t5 was of -0.3, then it can smoothly change current pos_x=pos_x-0.3
```

Important to understand, reconciliations should be normalized, meaning, smooth. Regarding space constraints, I use the example of space and movement because is what is easier to assimilate for this type of continuous games, but it could be a completely different game, which doesnt has movement, but still, if there's a continuous action in the FE which is restricted by some constraints, then those constraints whichever they are need to exist in both FE and BE, because BE needs to know about them in order to decide whats allowed and not, but FE also should know so it can use its own prediction instead of waiting for BE state, BE returned state should ideally be for reconciliation only, which shouldnt be much needed.

And now, there;s the other scenario, how player sees other players state, in that case, is pretty different, because for own changes it predicts base on its inputs, for other players they wont receive the other players inputs, just the state, the state that they receive is part of the players projection, I mean, each player will have its own projection, is that projection the one that contains the others players state, whcih the BE decides what part of their states to show, so projection could be like:

```
{
  "current_pos_x" : 45.0,
  "current_pos_y" : 78.0,
  "powers" : [
    {
      "name" : "fire",
      "damage_level" : 90,
      "remaining_capacity" : 78.3
    }
  ],
  "health" : {
    "stamine" : 34,
    "shield" : 10
  },
  "team" : {
    "name": "red".
    "team_players_count" : 3,
    "remaining_alive_players" : ["josue_934", "juan3445"]
  },
  "players" : [
    {
      "name" : "josue_934",
      "current_pos_x" : 12.0,
      "current_pos_y" : 45.0
    },
    {
      "name" : "juan3445",
      "current_pos_x" : 49.0,
      "current_pos_y" : 34.0
    },
    {
      "name" : "evil_guy45",
      "current_pos_x" : 20.0,
      "current_pos_y" : 37.0
    }
  ]
}
```

so in the example above, the players projection shoes its own information of the game, and then it has a players list, which is what shows the other players state, and thats part of the players projection, and in this case it only shows their positions, the game doesnt consider that a player must know anything else, like their powers, health, etc (this is just for a random example), in this case just for this example there's a list which is part of the players styate which tells what players are part of his team, meaning all other players are not part of the team, like evil_guy45.

Now, we'd also have predictive behavior for other players, bu tits important to understand and accept that what I see from the other players will always have lag, because for them we;ll always be some samples in the past, as we;ll be renderfing based on what we receive from the BE state, which as already understood before is stale data, how old is it depends on how often we want to process ticks, if we process ticks every 50ms, and lets assume a 30ms operational latency (processing time + network latency) then we'd always be seeing 80ms in the past for the other players, which is acceptable for the type of games that this platform aims to.

In order to achieve the predictive behavior FE can store for each other players the list of the last X states, so that it can normalized the changes and predict whats happening next, so if last states have beend ecreasing the speed, then it can continue with that acceleration evenn before receiving the next sample, if next sample shows that that player actually started increasing acceleration within that tick, then it reconciles.

Another thing is how authoratice qwe want the BE to be, that depends on the game scripts design, but we can decide if we want to stick with a single approach and provide built in functions/libraries for that, the other option could be not storing the list of latest players states, that option assumes that FE is the one predicting what to do based on the history, meaning, FE is calculating speed, acceleration, etc. the other option would be to have BE state always sending that info, so FE only uses last state, BE state would also tell, for example, for a movement game, it would tell the current speed and acceleration of that player, the only responsability of the FE is to smoothly change to that speed, so if FE had that player 2 speeed in x was 9.8, and then it just received a new BE state with speed 3.4, then the FE responsability is to simply go from 9.8 to 3.4 smoothly, and after that, it'll keep the 3.4 till it gets new speed, or even, the BE could send current acceleration, so even after reaching 3.4, if it has that acceleration was 0.3, then it continues applying that acceleration to the 3.4, lets say that for next BE received state it already had the speed in x at 1.2 since it was applying the acceleration, and BE tells that is actually 1.7 and acceleration changed to 0.4, then FE reconciles but now reconciliation is smaller that what would be if it had to go from 3.4 to 1.7, all this is taking into consideration a movement game, but again, then this reconciliation mechanism would need to be enforced also for every FE-BE communication, its not always about position.

Another example I can think of, is lets say a game where you click buttons to send power attacks, but the capability of powers you have depend on stuff, then it would be the same, the BE would be sending values which Fe uses to predict how much power is it expected to have as capability, I dont know, lets imagine, how many attacks of fire left

```
  initial_fire_attacks_capability: 1000
  s1 FE 240 attacjks
  s2 FE 230 attacks
  s3 FE 390 attacks
  BE receives first 3 samples and processes, determines that new fiee capability is 936, and tells FE "fire attack capability is decreasing at a rate of X"
  Then FE continues sending attacks based on teh new 936 capability but it dinamically changes its value base on its prediction
  NExt BE processign BE tells a new rate and capability
```

This example is a random example where the capability of attacks is some internal calculation, is not capability - fire attacks, lets imagine it does a lot of thinks to calculate the capabiklity.
What could happen is that FE bnecause of its prediction for a few ms doesnt allow player to send attacks because it thinks it reached the max, but then BE tells that player actually has more attacks, which is fine, is just a few ms, or the other way around, FE allows player to press button to send fire attacks, even though player couldnt, is fine, for a few ms the player clicked fire attacks which had no actual effect.

- Now that I mention this, BE should assume that FE can send not allowed samples, because FE might think that an operation was allowd even though it want, backend script should be smart to understand this and simply ignore them as they are not allowed and have no effect, is not an error, is expected

I think I changed the topic, I was erxplaining about how we see other players changes, but I ended writing an example of a player with its own state, but its also what I wanted to say about other players, what could happen with this approach is a player seeing other players sending fire attacks still even though they are not or they actually have no more capability, its fine, we assume that if an operation is a sample and not a command then its continuous and its no problem if we're inconsuistent for a few ms.

FINAL NOTE: this was the last section wirtten, so other places might refer to different mechanism, separating impulse and sample, and stuff, but this was the latest section written, so assume that everything here represents my actual current idea of how it should work

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

- just thought of the following, havent integrated it in the described flows, but we'll have to differentiate between mobile and desktop support, if mobile then game only tracks finger related events and UI buttons and stuff, whereas if its desktop then we support mouse and keyboard input, for the ticks, its very important, so a game version will be marked with the allowed device type (if the game only uses mouse/Ui buttons then it can be played in both desktop and mobile) and we'd have to receive that context in the join endpoint to determine if player can join or not
