# game pkg

This pkg only contaisn the implementations for the game definition language and their runtime engines

Pkg is organized in folders by version, like `v1` `v2`

Inside each folder, there should be:

- A single `program` pkg which contains the game definition language
- One or more `engine*`, which implements the runtime behavior based on the definition language, there could be many engines, but normally we'd only need one, however, the ideal architecture is:
  - That a game definition language is an isolated pkg which doesnt care about how its going to be processed at runtime, it only cares and owns the logic of defining some game through a language (and encoding/decoding it), it shouldnt depend on its engine, thats why theorically there could be more than 1 engine, there could be as many implementations as we want that use the defined language and treat it at runtime as they prefer

The `engine`s pkg only care about "compiling" and then processing the language, they dont care about the actual external interaction, they should simply expose an API which then external consumers can use, for instance:

- There could be a consumer which uses the engine for an online web platform, that consumer implements the UI instructions in the web, it implements the backend communication through websocket, etc
- There could be another consumer which uses engine for a cmd application, it implements the UI instructions in the cmd as it prefers (a container might be represented literally textually, thats up to the consumer), and the communication would be sync through cmd inputs

Ideally the `engine` implementations would be stateless and based on receiving input state and returning new state, that way is the consumer the one that owns everything and only uses the engine for the exposed behavior, but this is just the ideal approach, however as said, there could be many engine implementations for every language, so if a dev wants, they could implement a statefull engine, as long as it doesnt deal with external communication its permitted.
