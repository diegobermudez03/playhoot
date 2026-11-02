import { defineGame } from '@playhoot/game-server';

import { commands, type View } from '../shared/actions';

// First to GOAL taps wins; when time runs out, whoever leads.
const GOAL = 20;
const DURATION_MS = 30000;

type State = { taps: Record<string, number>; winner: string | null };

export default defineGame({
  players: { min: 1, max: 8 },
  commands,
  setup: (players): State => ({
    taps: Object.fromEntries(players.map((player) => [player, 0])),
    winner: null,
  }),
  on: {
    tap(state, { player, finish }) {
      state.taps[player] += 1;
      if (state.taps[player] >= GOAL) {
        state.winner = player;
        finish();
      }
    },
  },
  update(state, { now, finish }) {
    if (now < DURATION_MS) return;
    state.winner = leader(state);
    finish();
  },
  view: (state, player, { now }): View => ({
    me: player,
    taps: state.taps,
    goal: GOAL,
    secondsLeft: Math.max(0, Math.ceil((DURATION_MS - now) / 1000)),
    winner: state.winner,
  }),
});

function leader(state: State): string | null {
  let best: string | null = null;
  for (const [player, taps] of Object.entries(state.taps)) {
    if (best === null || taps > state.taps[best]) best = player;
  }
  return best;
}
