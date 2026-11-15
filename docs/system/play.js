// Domain: play (parent of session and runtime).
PH.domain({
  id: 'play', parent: null, kind: 'traditional', status: 'implemented',
  name: 'Play',
  summary: 'Executable game versions and Sessions: durable state in session, live execution in runtime.',
  responsibilities: [
    'Hold the content of each game version and whether it can run.',
    'Run Sessions from lobby to end.'
  ],
  doesNotKnow: ['Game cards, owners and visibility (Game).', 'Accounts (Identity): it receives user identifiers only.'],
  sources: ['P15§2']
});
