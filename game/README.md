# Game

Status: CANONICAL DOMAIN MODEL

## Responsibility

Game owns the authored lifecycle and runtime execution of Playhoot games.

## Owns

- Authored games and their definitions/versions.
- Game metadata and visibility/publication state.
- Live game sessions.
- Session runtime state and participation.
- Execution lifecycle of a game session.

## Does Not Own

- Transport/network connections.
- Identity/profile ownership.
- Public discovery/search experience.

## Internal Structure

### Business Capabilities

- Game Management: owns authored game lifecycle concerns.
- Session Runtime: owns execution/session lifecycle concerns.

### Supporting Subsystems

- Game Language: defines, compiles, and executes game behavior for Game; it is not a business domain.

## Boundary Notes

Completed-session history/archive ownership remains unresolved and is not defined by this document.
