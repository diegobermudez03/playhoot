# Backend Script Authoring Constraints

Status: CURRENT IMPLEMENTATION

This document is for whoever - human or AI - writes a Game's backend script:
the JavaScript source executed inside Session Runtime's sandbox to compute a
game's next state and requested commands. It states what that sandbox
actually supports and why, at the level an author needs, not the sandbox's
own internal implementation. The deeper technical contract lives in
`session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md`; this document
does not duplicate it.

A backend script is validated against the rules below before it is accepted
for publish or use (`session/usecases/scriptlint`). A violation of a "not
supported" rule below rejects the script outright. A "safe, but behaves
differently than you might expect" rule below does not reject the script,
but is worth reading before relying on the behavior it describes.

## Not Supported

Your script runs with no access to the outside world: no network, no
filesystem, no environment variables, no child processes, and no ability to
read anything about the machine it runs on. Referencing any of the
following fails validation, and would fail at runtime anyway if it somehow
reached it:

- Network access of any kind (`fetch`, `XMLHttpRequest`, `WebSocket`, or
  anything similar).
- Reading or writing files (`fs`, or the sandbox's own low-level `std`/`os`
  modules).
- Environment variables, process control, or anything else that reads real
  information about the host machine (`process`, `navigator`,
  `performance`).

If your game logic seems to need one of these, it needs a different
mechanism than "the backend script does it directly" - talk to whoever owns
the platform's command vocabulary about what your script should request
instead.

## Safe, But Behaves Differently Than You Might Expect

These do not fail validation and are safe to use, but do not behave like a
normal JavaScript environment because your script does not run against the
real system clock, real randomness, or a real event loop:

- **`Math.random()`** does not return real randomness. It returns a value
  deterministically derived from this specific execution's own random seed
  - calling it twice with the exact same inputs always produces the exact
    same sequence. This is intentional: your game's outcome must be
  reproducible from its own recorded inputs.
- **`Date`, `Date.now()`, and `new Date()`** do not return the real wall
  clock. They return a value derived from this execution's own logical
  time - the moment the platform recorded the triggering event, not the
  moment your code happens to run. Do not use these to measure real elapsed
  time or wall-clock deadlines.
- **`setTimeout`, `setInterval`, and `queueMicrotask`** are accepted, but
  their callback never actually runs: your script's result is captured the
  moment its `execute` function returns, and the sandbox shuts down
  immediately afterward - there is no later moment for a deferred callback
  to fire in. If your game needs something to happen later, request a timer
  through the platform's own command vocabulary instead of scheduling one
  yourself.

## Why A Static Check, Not Just The Sandbox Itself

The sandbox already prevents any of the "Not Supported" items above from
doing real harm even if your script somehow referenced them - there is
nothing behind them to reach. Catching them before your script is accepted,
rather than only at the moment a real session hits that code path, gives
you the failure immediately and with a clear reason, instead of a generic
runtime error discovered later, possibly by a real player.

This check reduces surprises; it does not, and cannot, guarantee your
script behaves identically every time it runs. JavaScript is a
general-purpose language, and a static check cannot see through every
possible way to construct a call indirectly (`eval`, for example, remains
usable, because banning it here could not stop a script from working
around the ban anyway). Write your script assuming this check catches
common mistakes, not that it makes your script provably deterministic.
