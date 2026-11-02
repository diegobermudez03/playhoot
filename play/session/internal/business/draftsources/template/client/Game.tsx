import { useCommand, usePlayer, useView } from '@playhoot/game-react';

import { commands, type View } from '../shared/actions';

export default function Game() {
  const view = useView<View>();
  const { players } = usePlayer();
  const tap = useCommand(commands.tap);
  if (view === null) return null;

  const mine = view.taps[view.me] ?? 0;
  const done = view.winner !== null;
  return (
    <div style={styles.screen}>
      <div style={styles.header}>
        <span>Goal: {view.goal}</span>
        <span>{done ? 'Finished' : `${view.secondsLeft}s`}</span>
      </div>
      <button style={styles.button} disabled={done} onClick={() => void tap()}>
        {done ? (view.winner === view.me ? 'You win!' : 'Game over') : mine}
      </button>
      <ol style={styles.board}>
        {players.map((player) => (
          <li key={player} style={player === view.me ? styles.me : undefined}>
            {player === view.me ? 'You' : player}: {view.taps[player] ?? 0}
            {view.winner === player ? ' 🏆' : ''}
          </li>
        ))}
      </ol>
    </div>
  );
}

const styles = {
  screen: {
    height: '100%',
    display: 'grid',
    gridTemplateColumns: '2fr 1fr',
    gridTemplateRows: 'auto 1fr',
    gap: 48,
    padding: 64,
    boxSizing: 'border-box',
    background: '#14213d',
    color: '#fff',
    font: '600 48px system-ui, sans-serif',
  },
  header: { gridColumn: '1 / 3', display: 'flex', justifyContent: 'space-between' },
  button: {
    borderRadius: 48,
    border: 'none',
    background: '#fca311',
    color: '#14213d',
    font: '800 160px system-ui, sans-serif',
    cursor: 'pointer',
  },
  board: { margin: 0, display: 'flex', flexDirection: 'column', gap: 24 },
  me: { color: '#fca311' },
} as const;
