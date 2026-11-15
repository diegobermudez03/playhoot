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

PH.operation({
  id: 'play.runtime.live-connection', domain: 'play.runtime', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'AttachAdmin / AttachPlayer / TerminateSession / Close',
  summary: 'One controller goroutine per live Session; admits a connection and drives it until it ends.',
  reads: [], writes: [],
  rules: [
    'Lobby admission, samples, commands, start, 100 ms ticks, retries, checkpoints every ≥5 s, reattach/rejoin/leave, disconnection with 3 s transport grace.',
    'TerminateSession exists but nothing calls it yet.',
    'Participant presence (active/reconnecting/disconnected/left) is in-memory only, never persisted.'
  ],
  limits: [
    { what: 'Live Sessions per instance', value: '500 (server_busy beyond it)', source: 'domain' },
    { what: 'Admins per Session', value: '8', source: 'domain' },
    { what: 'Samples per participant', value: '120/s and 3600/min', source: 'domain' },
    { what: 'Commands per participant / Session', value: '10/s,120/min / 60/s,600/min; 4 pending per participant', source: 'domain' }
  ],
  sources: []
});

PH.operation({
  id: 'play.runtime.previews', domain: 'play.runtime', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'StartPreview / AttachPreview',
  summary: 'Live connection to a preview of a build; no students, no grades.',
  reads: [], writes: [],
  rules: ['At most 50 previews live per instance, 10 min idle before ending.'],
  sources: []
});

PH.operation({
  id: 'play.runtime.ownership', domain: 'play.runtime', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'LocateOrClaimOwner / ReleaseOwner',
  summary: 'Decides which instance owns a live Session (placeholder: always this instance, fixed epoch 1).',
  reads: [], writes: [],
  rules: ['ReleaseOwner does nothing today; only a single instance is actually supported.'],
  sources: []
});

PH.operation({
  id: 'play.runtime.simulate', domain: 'play.runtime', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'Simulate / Result / ReadState',
  summary: 'Plays a scenario on a virtual clock over a build\'s script, without a live controller.',
  reads: [], writes: ['play.versions.play_simulation_results'],
  rules: ['Scripted expectations and holds; no participants, no replay.'],
  limits: [
    { what: 'Simulation budget', value: '10 min game time, 2000 steps, 50 observations, 10 s real time, 8 MiB, 2 at a time per instance', source: 'domain' }
  ],
  sources: []
});

PH.operation({
  id: 'play.runtime.limit-audit', domain: 'play.runtime', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'Hit / Run / Flush',
  summary: 'Counts live limit hits in memory and flushes them to the session\'s fault-report audit every 5 s.',
  reads: [], writes: ['play.session.play_game_fault_reports'],
  rules: [], sources: []
});
