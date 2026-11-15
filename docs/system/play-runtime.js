// Subdomain: play.runtime. Detail is loaded by T-004 (existing) and the H5 design (rest).
PH.domain({
  id: 'play.runtime', parent: 'play', kind: 'traditional', status: 'implemented',
  name: 'Play runtime',
  summary: 'Live execution: one controller per Session, the script sandbox, per-player views, simulation and replay feed.',
  responsibilities: [
    'Run the game script in the isolated sandbox; the server is authoritative.',
    'Admit players, relay commands and samples, project per-player views with deltas, and keep checkpoints.',
    'Keep a single active control per player ("Continue here" transfers it).',
    'Apply host perspectives: a host who plays sees only their own view; one who directs sees any.',
    'Recover a Session from the last reliable state after a platform failure, without counting the 60 s during the outage.',
    'Run simulations with scenarios, and previews without students or grades.',
    'Feed the replay recording while the game runs.'
  ],
  doesNotKnow: ['How Sessions are created or stored (play.session).', 'Why a player may join (decided before reaching it).'],
  sources: ['B: play', 'P06', 'P09§4']
});

PH.domain({
  id: 'play.runtime.sandbox', parent: 'play.runtime', kind: 'process', status: 'implemented',
  name: 'Sandbox',
  summary: 'A separate process that executes the untrusted backend script of a game; runtime drives it.',
  responsibilities: [
    'Execute game scripts isolated from the platform process.',
    'Hold the in-memory game state the script works on, mirrored with runtime.'
  ],
  doesNotKnow: ['Everything outside the script: no network, no tables, no identities beyond what runtime passes.'],
  sources: ['B: play (sandbox)']
});
