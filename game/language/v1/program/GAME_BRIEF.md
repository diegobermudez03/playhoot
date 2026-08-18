Wrote by AI, human dev notes added as `Dev note:`

# Game brief template (for the human describing a game)

This is the structured form a human fills out to describe a game they want built. Filled out, it's combined with `DEFINITION.md` in one prompt to an AI, which then produces a `program.Definition` (see `README.md`) or explains why the description isn't buildable as described.

The blocks exist so that (a) the human gives everything the AI actually needs, not just a vague pitch, and (b) the AI has unambiguous answers to the specific questions that map directly onto this platform's model (turns, state, actions, prompts, UI, assets) instead of having to guess. **Answer every block.** If a block genuinely doesn't apply to your game, write "N/A" and say why — don't just skip it, since a skipped block is indistinguishable from a forgotten one.

If you don't know the answer to something technical (e.g. "should this be a task group or one child per player?"), describe the *intent* in plain language instead and let the AI figure out the mechanism — just don't leave the intent itself vague.

---

## 1. Overview

- **Title / working name:**
- **One-paragraph pitch** (what is this game, in plain language, as if describing it to a new player):
- **Number of players:** (a fixed number, a range, or "any number ≥ N"?)
- **Session shape:** does one game instance run start-to-finish once (like a single match), or can it be paused and resumed across multiple sittings? Is there a lobby/waiting period before it starts?

## 2. Objective and end conditions

- **How does a player (or the game) win?**
- **How does a player (or the game) lose?** Can the game end in a draw, a forced abandonment, or a "no winner" state?
- **What ends the whole game instance** — one player reaching a condition, all players finishing, a fixed number of rounds, a timer, or something else?
- **What happens after the game ends** — is there a results/summary screen, a score breakdown, anything players see once it's over?

## 3. Setup

- **What has to be true the moment the game starts** — initial scores, an initial board/hand/deck state, starting positions, roles assigned, etc.?
- **Is any of this randomized at setup** (shuffled deck, randomly assigned roles, a rolled starting value)? Say exactly what's randomized and from what set of possibilities.
- **Does setup need any input from players before the game can begin** (choosing a name, a color, a role, an initial bet)?

## 4. Turn / flow structure

- **Is this turn-based (one player acts at a time, in some order), simultaneous (everyone acts, then it resolves together), free-form (anyone can act anytime), or a mix?**
- **If turn-based:** what decides turn order, and what advances it (a fixed rotation, a rule-based skip, a player choosing who's next)?
- **What can happen "out of turn"** — is there anything a player can always do regardless of whose turn it is (chat, forfeit, react to something)?
- **Rounds/phases:** does the game have distinct phases (e.g. "bidding phase" then "playing phase") with different rules for what's allowed in each? Describe each phase and what's different about it.

## 5. Game state to track

List every piece of information the game needs to remember while it's running. For each:

- **Name and meaning** (e.g. "each player's score", "the deck", "whose turn it is")
- **Shared (everyone can know it) or private (only specific players can know it)?** — remember: there's no engine-level "private storage"; anything private still lives in shared state and is hidden from other viewers only through what each player is shown (see block 8). State it as private here regardless — that's the design intent the AI needs, even though the mechanism is "shown selectively," not "stored separately."
- **How it changes** (what actions or events update it, and how)

## 6. Player actions

For every distinct thing a player can actively choose to do, give:

- **Name** (e.g. "Roll", "PlayCard", "Bid", "Forfeit")
- **When it's valid** (any time? only on your turn? only in a specific phase? only if some condition holds?)
- **What information it needs from the player** (parameters — e.g. "PlayCard" needs which card; "Bid" needs an amount)
- **What it does** (how state changes, what happens next)

## 7. Prompts to players (questions the game asks)

Some games need to explicitly stop and wait for a specific answer from a specific player, distinct from "the player can act whenever they want" (block 6). For each such prompt:

- **What's being asked, and to whom**
- **What kind of answer is expected** (yes/no, a number in some range, a choice from a list, free text)
- **Any rule for what counts as a valid answer** beyond its basic type (e.g. "must be between 1 and the pot size")
- **What happens if they never answer** — is there a timeout, or does the game just wait indefinitely?

## 8. Group interactions

If the game ever needs to ask **multiple players at once** and wait for their responses together (not one at a time):

- **Who's asked, and what**
- **What triggers moving on**: does it need *everyone* to respond, just the *first* response, or some *quorum* (e.g. "majority", "at least 3")?
- **What happens to players who don't respond** in time, if there's a cutoff?

## 9. Randomness

List every place the game involves chance (dice, shuffles, random draws, random selection) and, for each, exactly what set of outcomes is possible and with what shape (a uniform range, a shuffle of an existing list, picking one random element from a list).

## 10. Timers and time limits

- **Does anything in the game expire or auto-resolve if a player takes too long?** For each: what's the time limit, and what happens when it's reached (auto-pass, auto-lose, a default choice)?
- **Are there any other time-based effects** (a countdown shown to players, a phase that ends automatically after a duration)?

## 11. Sub-processes / parallel structure

If any part of the game is naturally its own independent sub-process — a mini-game inside the main game, something that runs once per player in parallel, a delegated task with its own outcome that the main game waits on — describe it:

- **What is it, and what triggers starting one**
- **Is there exactly one at a time, one per player, or a runtime-determined number of them?**
- **What outcome does it report back, and what does the main game do with that outcome?**
- **Does the main game need to wait for *all* of them, just the *first*, or some quorum, before continuing?**

## 12. UI and screens

For every distinct thing a player sees on their screen:

- **Screen/area name** (e.g. "main board", "your hand", "a popup asking you to confirm")
- **Who sees it** — everyone, only the current player, only specific players, only whoever's being asked a prompt?
- **What information does it show** — be specific about which pieces of state from block 5 appear here, and note when something shown is derived/computed rather than raw state (a total, a ranked list, a filtered view of something private that others can't see)
- **What can the player do from this screen** — which of block 6/7's actions/prompts are triggered from here, from which visible element (a button, tapping a card, etc.)?
- **Does it update live as state changes**, or only when explicitly re-shown?

## 13. Assets

List every image, icon, sound, or other external asset the game needs, each with a **short, stable, unique id** you're introducing right now (e.g. `dice_face_1` through `dice_face_6`, `card_back`, `chime_correct`). Include what it actually is/looks like/sounds like in enough detail for whoever produces the actual asset later. The AI building the game definition will reference assets **only by the id you give here** — it will never invent a file path, a URL, or its own id. If you don't have ids yet, at minimum list what's needed; expect to be asked to assign ids before generation.

## 14. Edge cases and clarifications

Anything you already know is ambiguous, a special case, or a rule you want to make sure isn't glossed over (what happens on a tie, what happens if a player disconnects, what happens if an action is attempted at an invalid time, etc.).

## 15. Explicitly out of scope

Anything you want to be clear is *not* part of this game, especially if it's the kind of thing someone might otherwise assume (no real-money stakes, no persistent player accounts across different games, no voice chat, etc.) — this prevents the AI from over-building or making assumptions in a direction you didn't intend.

---

## A note on feasibility

This platform builds **deterministic, signal-driven, turn/step-based games** — not continuous simulations. If your description above turns out to need continuous real-time motion or physics, an always-on process unrelated to any player action or timer, client-side authority over a rule, or live external data mid-game, expect the AI to flag that specific part as not buildable and either propose a discrete equivalent (e.g. "roll to move" instead of watching a piece slide) or ask you to redefine that part — rather than silently producing something that only approximates what you asked for.
