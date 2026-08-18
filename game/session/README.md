## session pkg

This session pkg is one layer above the language pkg in logical terms (even though the actual pkg is at the same level of the language pkg).

What I mean by logically one layer above is that it depends on the `language` pkg for the compiled game, as it executes it, but it adds the stateful layer to that executable game.

This pkg owns:

- Persiting the session state
- Persisting the session history once session is completed (archive)
- Receiving events and matching them to the persisted session and execute the events

This pkg doesnt own:

- Communication layer (websocket, grpc, tcp, etc)
