// Domain: game (Game Management). Existing detail is loaded by T-004; the planned part by milestone designs.
PH.domain({
  id: 'game', parent: null, kind: 'traditional', status: 'implemented',
  name: 'Game',
  summary: 'The logical game: card, owner, visibility, reuse permission, classification, and the catalog Discovery searches.',
  responsibilities: [
    'Keep the game card: name, description, cover image, visibility, academic mark, one category and tags.',
    'Know who owns each game and apply holding and creation limits.',
    'Open editions; hold which version + card combination is approved for Discovery (the review decision belongs to moderation).',
    'Own the template permission and its provenance and credits chain; keep the featured-template selection.',
    'Record the private-content declaration of each game.',
    'Answer Discovery and search: popular, categories, tags, language, template filter, text; approved content only.',
    'Decide who may play or edit a game (visibility, ownership).',
    'Record creations as they actually happen (its own usage count).'
  ],
  doesNotKnow: [
    'Scripts, assets and executable versions (play.versions) and Sessions (play.session).',
    'Classes and teacher libraries (classroom references games by identifier).',
    'Review queues and measures (moderation): game stores the resulting state, not the decision.',
    'Prices and allowances (plan passes capacity limits with each call).'
  ],
  references: [
    'Identity: the owner of each game (user identifier).',
    'Play: the version approved or offered, by version identifier.'
  ],
  sources: ['B: game', 'P04', 'P05', 'P14§2', 'P15§1']
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
