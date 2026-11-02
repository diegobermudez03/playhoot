import { defineCommands } from '@playhoot/game-core';

export const commands = defineCommands({ tap: {} });

export type View = {
  me: string;
  taps: Record<string, number>;
  goal: number;
  secondsLeft: number;
  winner: string | null;
};
