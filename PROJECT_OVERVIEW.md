# Project Overview

> This document describes the product vision and intended behavior of the project. It intentionally does not define implementation details, package boundaries, storage choices, runtime architecture, transport protocols, or deployment strategy.

## Summary

This project is an AI-assisted platform for creating, publishing, and playing multiplayer social room games.

A creator should be able to describe a game in natural language, explain its rules, provide or request visual assets, and receive a playable multiplayer experience without learning a custom programming language or manually assembling a large set of templates.

The platform is intended to support games whose identity comes primarily from rules, state, decisions, coordination, and interaction between players. Examples include card games, board games, trivia games, word games, hidden-role games, voting games, team games, negotiation games, turn-based strategy games, and other party or classroom experiences.

The long-term vision is:

> Describe the game you have in mind, refine it through conversation, and publish a reliable multiplayer version that other people can immediately join and play.

## The Problem

Existing ways of creating social multiplayer games usually fall into one of several categories:

1. Traditional programming tools provide flexibility but require substantial engineering and frontend knowledge.
2. No-code tools are easier to use but are often restricted to templates, rigid flows, or narrow game categories.
3. Visual rule editors can become complex enough that creators still need to learn a specialized programming model.
4. General AI code generation can produce quick prototypes, but those prototypes may be inconsistent, unsafe, difficult to validate, or unsuitable for reliable multiplayer sessions.

The project aims to provide both accessibility and expressive power.

Creators should communicate intent rather than implementation. The platform should translate that intent into a structured, playable game while enforcing the rules consistently and safely.

## Product Vision

The platform should make game creation feel conversational.

A creator might say:

> I want a four-player board game. Each player has four pieces. Players roll two dice, can capture enemy pieces, and must move all their pieces to a final lane to win.

The platform should then be able to:

- identify the important game concepts;
- detect missing or ambiguous rules;
- ask focused clarification questions;
- create the game rules;
- create the player-facing interface;
- provide temporary or generated visual assets;
- produce a playable preview;
- test common game scenarios;
- explain detected problems;
- apply later changes conversationally;
- publish a version that can be played in multiplayer rooms.

The creator should not be required to understand how the game is internally represented.

## Target Users

The platform may serve several types of creators:

- friends who want to invent a party game;
- teachers creating interactive classroom activities;
- streamers or community hosts creating audience games;
- event organizers creating custom group experiences;
- board-game enthusiasts prototyping original rules;
- families adapting traditional games;
- creators building games for public discovery;
- developers who want to prototype rule-heavy multiplayer games quickly.

The primary creation experience should not assume programming knowledge.

Advanced inspection and debugging tools may exist, but they should not be required for normal game creation.

## What Is a Social Room Game?

For this project, a social room game is a multiplayer experience where a known group of users joins a room and participates in a shared game session.

The game may be synchronous, turn-based, phase-based, or a combination of those models. Players may act individually, in teams, simultaneously, or in a controlled order.

Typical characteristics include:

- a limited number of participants;
- a shared session with a clear beginning and end;
- public and private information;
- explicit player decisions;
- rounds, turns, phases, timers, or deadlines;
- scoring, elimination, victory, or completion conditions;
- cards, boards, tokens, questions, resources, roles, or votes;
- different interfaces for different players;
- coordination between several human responses.

The platform is not initially intended to generate every possible kind of video game.

The initial product scope should favor games defined mainly by rules and state rather than games defined mainly by real-time movement, physics, combat, animation, or large 3D worlds.

## Intended Game Categories

The platform should eventually support games such as:

### Board Games

- race games;
- property and economy games;
- grid or path-based games;
- territory games;
- resource-management board games;
- dice-driven games;
- games with movable tokens and special cells.

### Card Games

- traditional deck games;
- custom card games;
- drafting games;
- collection and set-building games;
- team card games;
- games with hidden hands and public piles.

### Trivia and Word Games

- quizzes;
- finish-the-word games;
- answer-length games;
- category games;
- spelling or vocabulary games;
- collaborative classroom games.

### Social Deduction and Hidden Roles

- secret-role games;
- night and day phases;
- voting and elimination;
- asymmetric information;
- team objectives;
- private actions and public discussion.

### Party and Team Games

- simultaneous choices;
- team voting;
- challenges with shared scoring;
- prediction games;
- bluffing and negotiation;
- tournament or elimination formats.

### Narrative and Decision Games

- branching stories;
- group decisions;
- role-based scenarios;
- moral or strategic choices;
- collaborative problem solving.

These categories are examples, not hard-coded templates. The platform should support them through reusable capabilities rather than forcing every game into one predefined format.

## Core Product Concepts

### Creator

The person or group that defines, edits, tests, and publishes a game.

### Game

The complete authored concept: its rules, content, visuals, interactions, and expected player experience.

### Game Version

A published or saved revision of a game. Changes to a game should create a controlled new version rather than unpredictably changing sessions already in progress.

### Room

A place where users gather before or during a game. A room may be joined through a link, code, invitation, or discovery surface.

### Game Session

One concrete playthrough of one game version with a specific group of users.

### User

A real person connected to the platform or room.

### Participant

The role a user has inside a particular game. A participant may be a player, teammate, spectator, judge, host, controller, or another game-defined role.

A game may also contain roles or seats that temporarily have no connected user.

### Game State

The current authoritative facts of the session, such as:

- whose turn it is;
- where pieces are located;
- which cards each participant owns;
- current scores;
- active phase;
- remaining time;
- eliminated participants;
- selected answers;
- resources and inventories;
- winner or completion status.

### Interaction

A decision or action involving a user, such as:

- choosing a card;
- submitting an answer;
- selecting a board position;
- confirming an action;
- voting;
- making a trade proposal;
- pressing a game action button.

### Presentation

The client-visible representation of the current game for one user. Different users may see different information even while participating in the same session.

### Effect

A temporary presentation event, such as:

- a dice animation;
- a capture animation;
- confetti;
- a sound;
- a notification;
- a winner reveal.

Effects communicate what happened, but the game must remain correct even if a client misses one.

## Expected Creator Experience

### 1. Describe the game

The creator explains:

- the purpose of the game;
- the number of players;
- the rules;
- the objects involved;
- how players interact;
- how a turn or round works;
- how the game ends;
- the desired visual style.

The description may be incomplete or informal.

### 2. Clarify ambiguous rules

The platform should identify questions such as:

- What happens in a tie?
- Can a player skip a turn?
- What happens when a player disconnects?
- Is a response private or public?
- Can two pieces occupy the same position?
- What happens when a deck is empty?
- Does a timer use a default answer when it expires?

The system should ask only questions that materially affect the game.

### 3. Generate a playable draft

The platform creates an initial version containing:

- the game flow;
- the rules;
- the required interactions;
- the player-visible screens;
- placeholders or generated assets;
- basic presentation and feedback.

The first version should prioritize being playable and understandable over being visually perfect.

### 4. Preview and test

The creator should be able to:

- start a private test room;
- play with other people;
- simulate missing participants;
- inspect why an action was accepted or rejected;
- test specific scenarios;
- detect games that cannot progress or finish;
- review warnings about ambiguous or conflicting behavior.

### 5. Refine conversationally

The creator should be able to request changes such as:

- Make each player draw two cards instead of one.
- Add a thirty-second deadline to voting.
- Show eliminated players in gray.
- Let teams discuss before submitting a choice.
- Change the win condition to ten points.
- Move the action buttons to the bottom on mobile.
- Add a special card that reverses turn order.

The platform should apply controlled changes to the existing game rather than blindly replacing everything.

### 6. Publish and share

The creator publishes a game version and can create rooms from it.

Players should be able to join without installing development tools or understanding the creation system.

## Expected Player Experience

A player should be able to:

1. join a room through a simple invitation mechanism;
2. understand the game objective and current phase;
3. see only the information they are allowed to know;
4. receive clear available actions;
5. submit decisions through a purpose-built interface;
6. observe the shared game state update consistently;
7. receive feedback for invalid or unavailable actions;
8. reconnect when the game allows it;
9. see the final outcome and relevant scores or history.

The interface should adapt to the game and the user rather than presenting one universal template.

## AI Responsibilities

AI is the primary creation interface, but it is not merely a one-shot code generator.

The AI-assisted creation process should be responsible for:

- translating natural-language intent into precise rules;
- identifying entities, resources, roles, phases, and decisions;
- detecting ambiguity and contradictions;
- proposing sensible defaults;
- creating the player-facing experience;
- integrating uploaded or generated assets;
- creating tests and simulated scenarios;
- responding to validation feedback;
- repairing invalid game definitions;
- explaining game behavior in understandable language;
- applying focused edits to an existing game version.

The platform should not trust an AI-generated result solely because it looks plausible. Generated games should be checked by deterministic platform rules and simulations before publication.

## Expected Game Capabilities

The product should be able to express and enforce capabilities such as:

### Participants and Organization

- individual players;
- teams;
- seats;
- spectators;
- hosts or judges;
- hidden and public roles;
- bots or automated participants in the future.

### Information Visibility

- public shared information;
- private information per user;
- team-visible information;
- hidden roles;
- secret choices;
- delayed reveals.

A player must never receive authoritative secret information that should remain hidden from them.

### Game Flow

- lobby and setup;
- rounds;
- turns;
- phases;
- simultaneous actions;
- sequential actions;
- repeated cycles;
- branching outcomes;
- game completion.

### User Decisions

- single choice;
- multiple choice;
- selecting game objects;
- ordering items;
- voting;
- confirmation;
- free-text answers where appropriate;
- team decisions;
- simultaneous secret submissions.

### Coordination

- waiting for all participants;
- waiting for the first response;
- waiting for a required number of responses;
- deadlines;
- missing responses;
- cancellation;
- independent parallel activities;
- later aggregation of results.

### Rules and State Changes

- validation of legal actions;
- movement of cards, pieces, or resources;
- scoring;
- capture or elimination;
- inventory changes;
- ownership changes;
- random decisions;
- conditional rules;
- victory and failure conditions.

### Randomness

- dice;
- shuffled collections;
- random selection;
- random starting order;
- controlled random outcomes.

Random outcomes should be authoritative and reproducible for testing and replay purposes.

### Visual Experience

- boards;
- cards;
- tokens;
- text;
- images;
- buttons;
- lists;
- grids;
- overlays;
- modals;
- conditional views;
- per-user interfaces;
- transient visual and audio feedback.

## Correctness and Trust

A generated multiplayer game must do more than display an attractive interface.

The platform should ensure that:

- the same action is not processed twice;
- users cannot perform actions they are not allowed to perform;
- hidden information remains hidden;
- timers and late responses behave consistently;
- a game cannot silently enter an impossible state;
- simultaneous user actions resolve deterministically;
- failed changes do not partially corrupt a session;
- active interactions are cleaned up when phases or sessions end;
- game behavior can be inspected and explained;
- published versions remain stable.

## Explainability and Debugging

Creators should be able to understand why the game behaved in a particular way.

The platform should eventually answer questions such as:

- Why could this player not move that piece?
- Why did this team win the vote?
- Which rule sent this token back to jail?
- Why did the turn end?
- Which players failed to answer before the deadline?
- Why was this action rejected?
- What changed after the last decision?

A game should have an understandable history of important decisions and state changes.

## Example: Creating a Parques-Style Game

A creator describes a traditional race board game with:

- four players;
- four pieces per player;
- a shared circular route;
- player-specific exits and home lanes;
- two dice;
- rules for releasing pieces from jail;
- safe cells;
- capturing opponent pieces;
- extra turns for pairs;
- a penalty for three consecutive pairs;
- a penalty for missing an available capture;
- victory when all pieces finish.

The platform should be able to create:

- the board representation;
- the player pieces;
- turn management;
- authoritative dice rolls;
- valid movement choices;
- capture handling;
- safe-cell behavior;
- final-lane movement;
- per-turn controls;
- move previews;
- animations and feedback;
- winner detection.

Players see the board and interact with pieces and controls, but the platform consistently enforces the complete rules of the game.

The same general product capabilities should also support very different games without requiring a dedicated hard-coded Parques product mode.

## Product Differentiation

The project is not intended to be only:

- a collection of fixed game templates;
- a visual finite-state-machine editor;
- a general-purpose programming environment;
- a prompt that generates an isolated web application;
- a clone of a 3D game engine;
- a quiz builder with a few additional widgets.

Its intended differentiation is the combination of:

- natural-language creation;
- flexible rule-heavy multiplayer games;
- safe and constrained execution;
- automatically generated user interfaces;
- per-user information visibility;
- coordinated human interactions;
- deterministic validation and testing;
- conversational iteration;
- immediate room-based play.

## Product Principles

### Intent First

Creators describe what the game should do, not how it should be implemented.

### Multiplayer Native

Multiplayer behavior is a foundational capability, not an extension added after single-player creation.

### Rules Over Templates

Templates may improve speed, but they must not define the limits of what can be created.

### Correctness Before Automation

AI may generate content quickly, but the platform must enforce safety, consistency, and valid game behavior.

### Explicit Information Visibility

Every user receives only the state they are allowed to see.

### Explainable Behavior

The platform should be able to explain important decisions and rule outcomes.

### Playable Early

Creators should receive a functional draft before investing heavily in visual polish.

### Controlled Evolution

Published game versions should remain stable, while new versions can introduce deliberate changes.

### Creator Ownership

The creator remains responsible for the product idea, rules, style, and final decisions. AI accelerates and formalizes the work rather than replacing creator intent.

## Initial Product Boundary

The initial focus should be rule-driven social games with a moderate number of users and limited real-time physical simulation.

Strong initial candidates include:

- cards;
- boards;
- quizzes;
- word games;
- hidden roles;
- voting;
- teams;
- resources;
- turn-based decisions;
- simultaneous choices;
- social deduction;
- narrative choices.

The following are not initial priorities:

- open worlds;
- platformers;
- driving games;
- shooters;
- real-time combat;
- complex physics;
- high-frequency action games;
- large-scale 3D simulation;
- unrestricted user-authored code.

These boundaries may evolve, but the platform should avoid becoming a general-purpose game engine before proving its primary value.

## Preliminary Success Criteria

The product vision is validated when a creator without programming knowledge can:

1. describe an original social multiplayer game;
2. answer a limited number of clarification questions;
3. receive a functional playable draft;
4. invite other users into a room;
5. complete a session with consistently enforced rules;
6. make meaningful changes through conversation;
7. publish a stable revised version.

Technical sophistication alone is not sufficient. The generated game must be understandable, usable, and enjoyable enough for real people to play.

## Current Project Stage

The project is currently defining the foundational product model required to represent flexible multiplayer games safely and consistently.

Many product decisions are still open, including:

- the exact creator editing experience;
- asset generation and upload flows;
- content moderation;
- public discovery;
- monetization;
- marketplace behavior;
- persistent progression across sessions;
- supported room sizes;
- mobile and desktop presentation requirements;
- collaboration between multiple creators;
- analytics and creator dashboards;
- community remixing and sharing.

These open questions do not change the central product idea:

> A creator describes a social multiplayer game, the platform turns that intent into a validated and playable experience, and players join a room to play it together.
