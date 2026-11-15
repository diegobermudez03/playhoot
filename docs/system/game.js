// Domain: game (Game Management). Only what the create-session path needs; the rest is loaded by a later task.
PH.domain({
  id: 'game', parent: null, kind: 'traditional', status: 'implemented',
  name: 'Game',
  summary: 'The authoring life of a game: card, editions, publishing and visibility.',
  responsibilities: [
    'Keep the game card: name, description, images, visibility.',
    'Open editions and publish them.',
    'Decide who may play or edit a game.'
  ],
  doesNotKnow: ['Scripts, executable versions and Sessions (they belong to Play).'],
  sources: []
});

PH.table({
  id: 'game.games', domain: 'game', status: 'implemented',
  name: 'games',
  summary: 'One row per game: its card and who owns it.',
  purpose: 'The game as its owner sees it: card text, visibility and whether it was ever published.',
  columns: [
    { name: 'id', type: 'bigserial', note: 'Internal key.' },
    { name: 'uuid', type: 'uuid', note: 'External identifier used by other domains.' },
    { name: 'name', type: 'text', note: 'Up to 80 characters.' },
    { name: 'description', type: 'text', note: 'Up to 2000 characters.' },
    { name: 'owner_uuid', type: 'uuid', note: 'The owner (Identity user).' },
    { name: 'visibility', type: 'text', note: 'draft, private, hidden or public.' },
    { name: 'published_at', type: 'timestamptz', note: 'Set once, at the first publication.' },
    { name: 'template_allowed', type: 'boolean', note: 'Others may start games from it.' },
    { name: 'template_featured', type: 'boolean', note: 'Featured to AIs.' },
    { name: 'created_at', type: 'timestamptz', note: '' },
    { name: 'updated_at', type: 'timestamptz', note: '' },
    { name: 'deleted_at', type: 'timestamptz', note: 'Filtered on read.' }
  ],
  indexes: ['UNIQUE (uuid)'],
  history: 'Change table game_histories: name, description, visibility, published.',
  references: [{ column: 'owner_uuid', to: 'identity user' }]
});

PH.operation({
  id: 'game.check-playability', domain: 'game', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'checkplayability',
  summary: 'Says whether a person may start a Session of a game.',
  reads: ['game.games'],
  writes: [],
  rules: [
    'The game must be published and not deleted.',
    'A public game is playable by anyone; a private one only by its owner.'
  ],
  sources: []
});
