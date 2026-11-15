// Domain: play (parent of versions, session, runtime and replay).
PH.domain({
  id: 'play', parent: null, kind: 'traditional', status: 'implemented',
  name: 'Play',
  summary: 'Everything needed to run a game: version content, Session lifecycle, live execution and replays.',
  responsibilities: [
    'Hold the content of each game version and whether it can run (versions).',
    'Run Sessions from lobby to end and keep their durable state (session).',
    'Execute the live game in an isolated runtime and sandbox (runtime).',
    'Record, store and play back replays (replay).',
    'Keep the original result each participant produced, when the game delivers results.',
    'Record sessions as they actually happen (its own usage count).'
  ],
  doesNotKnow: [
    'Game cards, owners and visibility (game).',
    'Accounts (identity): it receives user identifiers only.',
    'Classes and the academic meaning of results (classroom).',
    'Plan capacities: limits arrive as parameters from the caller.'
  ],
  references: [
    'Game: the logical game each version belongs to (game identifier).',
    'Identity: hosts and participants (user identifiers).',
    'Library: platform assets a version uses (asset identifiers).'
  ],
  sources: ['B: play', 'P15§2', 'P06', 'P08§2']
});
