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
    { name: 'deleted_at', type: 'timestamptz', note: 'Set once; filtered on read.' }
  ],
  indexes: ['UNIQUE (uuid)', 'partial (owner_uuid) while not deleted'],
  history: 'Change table game_histories: name, description, visibility, published. Write-once: deleted_at.',
  references: [{ column: 'owner_uuid', to: 'identity user' }]
});

var EDITION_LIFECYCLE = {
  states: ['EDITING', 'PUBLISHED'],
  transitions: [{ from: 'EDITING', to: 'PUBLISHED', on: 'CompletePublication; the edition is then immutable' }]
};

PH.table({
  id: 'game.game_histories', domain: 'game', status: 'implemented',
  name: 'game_histories',
  summary: 'Every change to a game\'s name, description, visibility or published flag.',
  purpose: 'Append-only; creation records all fields, later rows only the ones that changed.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'name', type: 'text', note: 'Empty if unchanged.' },
    { name: 'description', type: 'text', note: 'Empty if unchanged.' },
    { name: 'visibility', type: 'text', note: 'Empty if unchanged.' },
    { name: 'is_published', type: 'boolean', note: 'Only on the row that changes it.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(game_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'game_id', to: 'game.games' }]
});

PH.table({
  id: 'game.game_editions', domain: 'game', status: 'implemented',
  name: 'game_editions',
  summary: 'An authoring edition of a game: a Play version under construction or already published.',
  purpose: 'Its uuid is the same as the Play version it corresponds to.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'uuid', type: 'uuid', note: 'Shared with the Play version.' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'status', type: 'text', note: 'EDITING or PUBLISHED.' },
    { name: 'template_game_uuid', type: 'uuid', note: 'Empty unless started from a template.' },
    { name: 'template_version_uuid', type: 'uuid', note: 'Empty unless started from a template.' },
    { name: 'published_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (uuid)', 'partial UNIQUE (game_id) while EDITING'],
  history: 'Change table game_edition_status_changes.',
  references: [{ column: 'game_id', to: 'game.games' }],
  lifecycle: EDITION_LIFECYCLE
});

PH.table({
  id: 'game.game_edition_status_changes', domain: 'game', status: 'implemented',
  name: 'game_edition_status_changes',
  summary: 'Every status an edition went through.',
  purpose: 'Append-only history.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'edition_id', type: 'bigint', note: '' },
    { name: 'from_status', type: 'text', note: 'Empty for the creation.' },
    { name: 'to_status', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(edition_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'edition_id', to: 'game.game_editions' }]
});

PH.table({
  id: 'game.game_create_requests', domain: 'game', status: 'implemented',
  name: 'game_create_requests',
  summary: 'Idempotency of game creation: one row per owner and key.',
  purpose: 'The same key with a different name or template is a conflict.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'owner_uuid', type: 'uuid', note: '' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'name', type: 'text', note: '' },
    { name: 'template_game_uuid', type: 'uuid', note: 'Empty unless started from a template.' },
    { name: 'game_id', type: 'bigint', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (owner_uuid, idempotency_key)'],
  history: 'Write-once: claimed and completed in one transaction.',
  references: [
    { column: 'game_id', to: 'game.games' },
    { column: 'owner_uuid', to: 'identity user' }
  ]
});

PH.table({
  id: 'game.game_card_images', domain: 'game', status: 'implemented',
  name: 'game_card_images',
  summary: 'The logo and photos currently displayed on a card.',
  purpose: 'Replacing an image removes the old row (removed_at) rather than overwriting it, which doubles as image history.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'kind', type: 'text', note: 'logo or photo.' },
    { name: 'sha256', type: 'text', note: '' },
    { name: 'object_key', type: 'text', note: 'games/card-images/<sha256>.' },
    { name: 'removed_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['partial (game_id, kind) while not removed'],
  history: 'Removed-not-overwritten rows are the history.',
  references: [{ column: 'game_id', to: 'game.games' }]
});

PH.table({
  id: 'game.game_card_objects', domain: 'game', status: 'implemented',
  name: 'game_card_objects',
  summary: 'Registry of card-image bytes, recorded before they are stored.',
  purpose: 'Standard object-storage intent record: register, then write bytes.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'sha256', type: 'text', note: '' },
    { name: 'deleted_at', type: 'timestamptz', note: 'Set once, by cleanup.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (sha256)'],
  history: 'Write-once: deleted_at.',
  references: []
});

PH.table({
  id: 'game.game_card_image_requests', domain: 'game', status: 'implemented',
  name: 'game_card_image_requests',
  summary: 'Idempotency of a card-image upload: one row per game and key.',
  purpose: '',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (game_id, idempotency_key)'],
  history: 'Write-once: claimed and completed in one transaction.',
  references: [{ column: 'game_id', to: 'game.games' }]
});

PH.table({
  id: 'game.game_releases', domain: 'game', status: 'implemented',
  name: 'game_releases',
  summary: 'A publication attempt for an edition: pending until completed or abandoned.',
  purpose: 'At most one pending release per game; sequence numbers the releases of a game.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'uuid', type: 'uuid', note: '' },
    { name: 'edition_id', type: 'bigint', note: '' },
    { name: 'sequence', type: 'int', note: 'Per game.' },
    { name: 'revision', type: 'bigint', note: 'The Play draft revision being published.' },
    { name: 'visibility', type: 'text', note: 'private or public.' },
    { name: 'owner_uuid', type: 'uuid', note: '' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'completed_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'abandoned_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'abandon_reason', type: 'text', note: '' },
    { name: 'languages', type: 'jsonb', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (game_id, sequence)', 'UNIQUE (game_id, idempotency_key)', 'partial UNIQUE (game_id) while pending'],
  history: 'Write-once: completed_at, abandoned_at, abandon_reason.',
  references: [
    { column: 'edition_id', to: 'game.game_editions' },
    { column: 'owner_uuid', to: 'identity user' }
  ]
});

PH.table({
  id: 'game.game_edition_requests', domain: 'game', status: 'implemented',
  name: 'game_edition_requests',
  summary: 'Idempotency of opening a new edition: one row per game and key.',
  purpose: '',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (game_id, idempotency_key)'],
  history: 'Write-once: claimed and completed in one transaction.',
  references: [{ column: 'game_id', to: 'game.games' }]
});

PH.operation({
  id: 'game.delete-user-games', domain: 'game', kind: 'usecase', effect: 'write', status: 'verified',
  name: 'DeleteUserGames',
  summary: 'Deletes every game a deleted user owns; a step of the account-cleanup of H3 (D-036).',
  reads: ['game.games'],
  writes: ['game.games'],
  rules: [
    'Sets deleted_at on each of the owner\'s games — the first writer of that column — in batches of 500.',
    'The deletion needs no history row: deleted_at is written once and is its own record (D-041).',
    'Existing Sessions and other people\'s history are untouched: ordinary deletion never interrupts them (P04§6).',
    'Idempotent: games already deleted are skipped. Single-game withdrawal and unpublication are H4\'s withdraw-game.'
  ],
  sources: ['P02§11', 'P04§6']
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

PH.operation({
  id: 'game.check-visibility', domain: 'game', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'checkvisibility',
  summary: 'Says whether a game is visible, by its visibility value alone (not wired into any caller today).',
  reads: ['game.games'], writes: [], rules: [], sources: []
});

PH.operation({
  id: 'game.find-editable-version', domain: 'game', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'FindEditableVersion',
  summary: 'Authorizes a user to edit a game and returns the version of its open edition.',
  reads: ['game.games', 'game.game_editions'],
  writes: [],
  rules: ['found only if there is an open edition and the caller is the owner; otherwise not_found or not_editable.'],
  sources: []
});

PH.operation({
  id: 'game.find-template', domain: 'game', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'FindTemplate',
  summary: 'Finds the most recent published version of a game allowed as a template.',
  reads: ['game.games', 'game.game_editions'],
  writes: [],
  rules: ['The game must be published, not deleted and template_allowed.'],
  sources: []
});

PH.operation({
  id: 'game.game-lifecycle', domain: 'game', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'CreateGame / BeginPublication / CompletePublication / AbandonPublication / OpenEdition',
  summary: 'Creates a game with its first edition, publishes an edition, or opens a new one after publication.',
  reads: ['game.games', 'game.game_editions', 'game.game_releases'],
  writes: [
    'game.games', 'game.game_histories', 'game.game_editions', 'game.game_edition_status_changes',
    'game.game_create_requests', 'game.game_releases', 'game.game_edition_requests'
  ],
  rules: [
    'CreateGame: one transaction creates a private, unpublished game with a first EDITING edition; idempotent by key, conflict if the same key names a different game or template.',
    'BeginPublication: at most one pending release per game; requires a name; only private or public visibility.',
    'CompletePublication: edition EDITING to PUBLISHED, games.published_at set once, release completed_at set with languages.',
    'OpenEdition: only on an already-published game with no open edition and no pending release; idempotent by key.',
    'visibility values are draft, private, hidden, public; only private and public ever get published.'
  ],
  limits: [
    { what: 'Pending release / open edition', value: '1 per game', source: 'domain' }
  ],
  lifecycle: EDITION_LIFECYCLE,
  sources: []
});

PH.operation({
  id: 'game.game-card', domain: 'game', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'gamecard (ListGames / ReadCard / UpdateCard / UploadImage / RemoveImage)',
  summary: 'Reads and edits a game\'s card: text and images.',
  reads: ['game.games', 'game.game_card_images'],
  writes: ['game.games', 'game.game_histories', 'game.game_card_images', 'game.game_card_objects', 'game.game_card_image_requests'],
  rules: [
    'UploadImage validates PNG, JPEG or WebP by bytes, cuts off by size, and records the intent before saving.',
    'Images are read back through a 15-minute signed URL from object storage.',
    'Idempotent by key per upload.'
  ],
  limits: [
    { what: 'Name / description', value: '80 / 2000 runes', source: 'domain' },
    { what: 'Photos per card', value: '8 (+1 logo)', source: 'domain' },
    { what: 'Image size', value: 'logo 2 MiB, photo 5 MiB', source: 'domain' }
  ],
  sources: []
});
