// Domain: orchestrator. Cross-domain writes; no tables of its own.
PH.domain({
  id: 'orchestrator', parent: null, kind: 'orchestrator', status: 'implemented',
  name: 'Orchestrator',
  summary: 'Coordinates writes that cross top-level domains; may persist the steps of flows that need recovery.',
  responsibilities: [
    'Call domains in order for operations that cross them; only it and composer may combine domains.',
    'Pass idempotency keys through; retries are completed by the domains.',
    'Fetch plan capacities and budget admission and pass them to the owning domain, which applies the stricter limit.',
    'Record the steps of flows that must recover or compensate (payments, grades), and finish incomplete ones without duplicating effects.'
  ],
  doesNotKnow: ['Tables of any domain.', 'Rules that belong to a single domain.'],
  sources: ['B: orchestrator', 'P15§6']
});

PH.flow({
  id: 'create-session', status: 'implemented',
  name: 'Create session',
  summary: 'Checks that the caller may play the game, then creates a Session in lobby with a join code.',
  coordinator: 'orchestrator', effect: 'write',
  steps: [
    { n: 1, domain: 'game', operation: 'game.check-playability', when: 'always', onFailure: 'Stop with game_unavailable.' },
    { n: 2, domain: 'play.session', operation: 'play.session.create-session', when: 'the game is playable', onFailure: 'Return its outcome: active_session_limit, game_unavailable or idempotency conflict.' }
  ],
  persistence: 'none',
  idempotency: 'The Idempotency-Key is passed to Play, which records each request once per user and key.',
  compensation: 'None needed: step 1 only reads.',
  sources: ['P04§3', 'P06']
});
