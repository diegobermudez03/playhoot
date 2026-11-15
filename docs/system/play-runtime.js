// Subdomain: play.runtime. Detail is loaded by a later task.
PH.domain({
  id: 'play.runtime', parent: 'play', kind: 'traditional', status: 'implemented',
  name: 'Play runtime',
  summary: 'Live execution: one controller per Session, the script sandbox, simulation and replay recording.',
  responsibilities: [
    'Run the game script in an isolated sandbox.',
    'Admit players, relay inputs and views, and keep checkpoints.',
    'Record replays.'
  ],
  doesNotKnow: ['How Sessions are created or stored (play.session).'],
  sources: []
});
