# STRUCTURE

- Global config:
    - There's a dynamic body obj `user_params`  which defines the initial user params, that means that for an user to join the session those user params must be sent, they can be empty, but in some games it can represent initial team selection, color, etc.
    - The behavior is divided between the server logic and the player logic
- Server:
    - contains the global state of the game, the globa, state always includes:
        - the list of users connected with their initial params which follows the same structure than the `user_params` and then the `state` which is dynamic and contains any user specific state
        - the list of current operations (ongoing operations):
            - each operation is identified by its id
            - each operation contains the operations specific state (if needed or if we want to store at operation level)
    - defines the events for the system:
        - an event can be a part of an operation, in which case i

