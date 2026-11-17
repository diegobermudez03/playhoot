// Domain: game (Game Management). Existing detail is loaded by T-004; the planned part by the H4
// design (T-400): publication as an approved combination, taxonomy, templates, Discovery search,
// usage aggregates and the administrative block.
PH.domain({
  id: 'game', parent: null, kind: 'traditional', status: 'implemented',
  name: 'Game',
  summary: 'The logical game: card, owner, publication, reuse permission, classification, and the catalog Discovery searches.',
  responsibilities: [
    'Keep the game card: name, description, cover image, academic mark, one category and tags.',
    'Know who owns each game and apply holding and creation limits.',
    'Open editions; hold which version + card combination is approved for Discovery (the review decision belongs to moderation) and whether the owner currently offers it.',
    'Own the template permission and its provenance and credits chain; keep the featured-template selection.',
    'Record the private-content declaration of each publication request.',
    'Answer Discovery and search: popular, categories, tags, language, template filter, text; approved and offered content only.',
    'Decide who may play or edit a game (ownership, offering, block) and which version a new Session of a caller pins (D-045).',
    'Keep the administrative block state moderation decides.',
    'Record creations as they actually happen (its own usage count) and keep the per-game usage aggregates play feeds it.'
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
    { name: 'published_at', type: 'timestamptz', note: 'Set once, at the first approved publication (re-specified by H4; today the first publish writes it).' },
    { name: 'category_id', type: 'bigint', note: 'H4 (planned): the one category of the working card; empty until chosen.' },
    { name: 'academic', type: 'boolean', note: 'H4 (planned): the academic mark, written by an approved publication (B: teacher games).' },
    { name: 'chosen_version_uuid', type: 'uuid', note: 'H4 (planned): the EXECUTABLE version the owner chose for new Sessions; empty = unavailable (P04§5). History in game_version_choices.' },
    { name: 'offered_publication_id', type: 'bigint', note: 'H4 (planned): the approved combination Discovery currently offers — the chosen version\'s approval if it has one, else the latest approved (P04§4); recomputed on approval, version switch and withdrawal (D-045). Empty = not public.' },
    { name: 'discovery_withdrawn_at', type: 'timestamptz', note: 'H4 (planned): the owner unpublished; cleared by re-offering. History in game_offering_changes.' },
    { name: 'blocked_at', type: 'timestamptz', note: 'H4 (planned): administrative block in force; cleared by unblock. History in game_blocks.' },
    { name: 'blocked_reason_code', type: 'text', note: 'H4 (planned): the current block reason kind; empty while not blocked.' },
    { name: 'popularity_score', type: 'int', note: 'H4 (planned): Sessions started in the last 7 days (V-13), recomputed hourly from game_usage_days; no history (derived).' },
    { name: 'created_at', type: 'timestamptz', note: '' },
    { name: 'updated_at', type: 'timestamptz', note: '' },
    { name: 'deleted_at', type: 'timestamptz', note: 'Set once; filtered on read.' }
  ],
  indexes: [
    'UNIQUE (uuid)', 'partial (owner_uuid) while not deleted',
    'H4 (planned): partial (popularity_score DESC, id) and (category_id, popularity_score DESC) while offered_publication_id IS NOT NULL, not blocked, not withdrawn, not deleted (the Discovery scan)'
  ],
  history: 'Change table game_histories: name, description, category, academic, published. Offering changes in game_offering_changes; blocks in game_blocks. Write-once: deleted_at. H4 drops the columns visibility, template_allowed and template_featured (migration edited in place, D-046/D-047): the publication rows, the template grants and the featured-template rows replace them.',
  references: [
    { column: 'owner_uuid', to: 'identity user' },
    { column: 'category_id', to: 'game.game_categories' },
    { column: 'offered_publication_id', to: 'game.game_publications' }
  ]
});

var EDITION_LIFECYCLE = {
  states: ['EDITING', 'PUBLISHED'],
  transitions: [{ from: 'EDITING', to: 'PUBLISHED', on: 'CompletePublication; the edition is then immutable' }]
};

PH.table({
  id: 'game.game_histories', domain: 'game', status: 'implemented',
  name: 'game_histories',
  summary: 'Every change to a game\'s name, description, visibility or published flag.',
  purpose: 'Append-only; creation records all fields, later rows only the ones that changed. H4 (planned): the visibility column goes with games.visibility; category and academic changes are recorded here instead (migration edited in place).',
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
  id: 'game.resolve-playable-version', domain: 'game', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'ResolvePlayableVersion',
  summary: 'Decides whether a caller may start a Session of a game and which version it pins (D-045). Replaces check-playability and check-visibility (T-401).',
  reads: ['game.games', 'game.game_publications'],
  writes: [],
  rules: [
    'A deleted or blocked game is not playable by anyone; a block refuses the owner too (P04§6).',
    'The owner plays the version chosen for new Sessions (games.chosen_version_uuid); none chosen answers game_unavailable.',
    'Anyone else plays only through the offered publication: its version, exactly as reviewed (P04§4). No offering, or withdrawn, answers not available.',
    'The answer carries the version uuid the Session must pin; play.session no longer resolves it (P15§2).'
  ],
  sources: ['P04§4-6', 'P15§2', 'D-045']
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
  summary: 'Finds the version a template copy starts from.',
  reads: ['game.games', 'game.game_editions'],
  writes: [],
  rules: [
    'The game must be published, not deleted and template_allowed.',
    'H4 (planned, T-404): the game must have a live template grant, an offered publication and no block; the copy starts from the offered publication\'s version, exactly as reviewed (P05§3). Reads game_template_grants and game_publications instead of the dropped boolean.'
  ],
  sources: ['P05§1-3']
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
    'BeginPublication: at most one pending release per game; requires a name.',
    'CompletePublication: edition EDITING to PUBLISHED, release completed_at set with languages.',
    'OpenEdition: only on a game with a finished version, no open edition and no pending release; idempotent by key.',
    'H4 (planned, T-401/T-404): a release finishes a version, it no longer makes anything public — games.published_at moves to the first approved publication, and the release\'s visibility column is dropped (D-045/D-046).',
    'H4 (planned, T-404): CreateGame receives the caller\'s plan limits from the orchestrator and applies the stricter of received and own: live games held (drafts included) against the saved-games capacity, creations in the rolling window (game_create_requests is the count) against the creations capacity; refusals game_limit_reached and creation_limit_reached (P27, D-049).',
    'H4 (planned, T-404): a creation from a template also writes the provenance row(s) in the same transaction (P05§3, D-047).'
  ],
  limits: [
    { what: 'Pending release / open edition', value: '1 per game', source: 'domain' },
    { what: 'Saved games (drafts included) / creations per rolling window', value: 'from the caller\'s plan (free: 3 and 2/30 days, P27§2)', source: 'plan' }
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
    'Idempotent by key per upload.',
    'H4 (planned, T-402/T-403): UpdateCard also takes the one category and the tags (from the allowed vocabulary); edits always change the working card only — what Discovery shows is the approved snapshot, and reaching it again means a new publication request (P04§2,§4, D-046). ListGames gains name search and category, tag and date filters (B: my games).'
  ],
  limits: [
    { what: 'Name / description', value: '80 / 2000 runes', source: 'domain' },
    { what: 'Photos per card', value: '8 (+1 logo)', source: 'domain' },
    { what: 'Image size', value: 'logo 2 MiB, photo 5 MiB', source: 'domain' }
  ],
  sources: []
});

// ---------- H4 (T-400 design): publication, taxonomy, templates, Discovery ----------

var PUBLICATION_LIFECYCLE = {
  states: ['SUBMITTED', 'APPROVED', 'DENIED', 'CANCELLED', 'SUPERSEDED'],
  transitions: [
    { from: 'SUBMITTED', to: 'APPROVED', on: 'the admin approves the combination (review-publication)' },
    { from: 'SUBMITTED', to: 'DENIED', on: 'the admin denies it with a note to the creator' },
    { from: 'SUBMITTED', to: 'CANCELLED', on: 'the game is deleted or the owner withdraws the request' },
    { from: 'SUBMITTED', to: 'SUPERSEDED', on: 'the owner submits a newer combination for the same game' }
  ]
};

PH.table({
  id: 'game.game_publications', domain: 'game', status: 'planned',
  name: 'game_publications',
  summary: 'One submitted version + card + assets combination; an APPROVED row is exactly what Discovery may show (P04§4).',
  purpose: 'The snapshot is frozen at submission: approving one combination approves nothing later, and Discovery never shows the working card.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'uuid', type: 'uuid', note: 'External identifier (moderation references it).' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'version_uuid', type: 'uuid', note: 'The EXECUTABLE version this combination offers.' },
    { name: 'status', type: 'text', note: 'See lifecycle.' },
    { name: 'card', type: 'jsonb', note: 'Snapshot: name, description, cover and photo object keys.' },
    { name: 'category_id', type: 'bigint', note: 'Snapshot of the one category.' },
    { name: 'academic', type: 'boolean', note: 'Claimed by the creator, confirmed or removed at review (B: teacher games).' },
    { name: 'private_content_declared', type: 'boolean', note: 'The mandatory declaration; true cannot be submitted (B: private content).' },
    { name: 'languages', type: 'jsonb', note: 'Version facts copied from play.versions at submission.' },
    { name: 'players_min, players_max', type: 'int, int', note: 'Version facts (manifest).' },
    { name: 'devices', type: 'text', note: 'desktop, mobile or both (version facts).' },
    { name: 'grade_support', type: 'boolean', note: 'Whether the version delivers results per participant; false until the H5 contract exists.' },
    { name: 'note', type: 'text', note: 'The admin\'s note on a denial, shown to the creator.' },
    { name: 'decided_by_user_uuid', type: 'uuid', note: 'Empty until decided.' },
    { name: 'submitted_at, decided_at', type: 'timestamptz x2', note: 'decided_at written once.' },
    { name: 'idempotency_key', type: 'text', note: 'Per game and key.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: [
    'UNIQUE (uuid)', 'UNIQUE (game_id, idempotency_key)', 'partial UNIQUE (game_id) while SUBMITTED',
    '(game_id, status, decided_at) to find the latest approved combination',
    'trigram on the snapshot name for Discovery text search (approved rows)',
    '(category_id) and (languages) over approved rows for the Discovery filters'
  ],
  history: 'Append-only rows plus the status change table game_publication_status_changes.',
  references: [
    { column: 'game_id', to: 'game.games' },
    { column: 'version_uuid', to: 'play.versions version (external identifier)' },
    { column: 'category_id', to: 'game.game_categories' },
    { column: 'decided_by_user_uuid', to: 'identity user' }
  ],
  lifecycle: PUBLICATION_LIFECYCLE
});

PH.table({
  id: 'game.game_publication_status_changes', domain: 'game', status: 'planned',
  name: 'game_publication_status_changes',
  summary: 'Every status a publication went through.',
  purpose: 'Append-only history.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'publication_id', type: 'bigint', note: '' },
    { name: 'from_status', type: 'text', note: 'Empty for the creation.' },
    { name: 'to_status', type: 'text', note: '' },
    { name: 'changed_by_user_uuid', type: 'uuid', note: 'Empty when the platform made the change.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(publication_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'publication_id', to: 'game.game_publications' }]
});

PH.table({
  id: 'game.game_publication_tags', domain: 'game', status: 'planned',
  name: 'game_publication_tags',
  summary: 'The tags frozen into a publication snapshot; the Discovery tag filters run here.',
  purpose: 'Written at submission from the working tags; never edited after.',
  columns: [
    { name: 'publication_id', type: 'bigint', note: '' },
    { name: 'tag_id', type: 'bigint', note: '' }
  ],
  indexes: ['UNIQUE (publication_id, tag_id)', '(tag_id, publication_id) for the filter'],
  history: 'Write-once with its publication (part of the snapshot).',
  references: [
    { column: 'publication_id', to: 'game.game_publications' },
    { column: 'tag_id', to: 'game.game_tags' }
  ]
});

PH.table({
  id: 'game.game_offering_changes', domain: 'game', status: 'planned',
  name: 'game_offering_changes',
  summary: 'Every time the owner withdrew the game from Discovery or offered it again.',
  purpose: 'Append-only history behind games.discovery_withdrawn_at; unpublish never touches the approval record (D-046).',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'offered', type: 'boolean', note: 'false = unpublished, true = offered again.' },
    { name: 'created_by_user_uuid', type: 'uuid', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(game_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'game_id', to: 'game.games' }, { column: 'created_by_user_uuid', to: 'identity user' }]
});

PH.table({
  id: 'game.game_blocks', domain: 'game', status: 'planned',
  name: 'game_blocks',
  summary: 'Every administrative block or unblock applied to a game, with its reason.',
  purpose: 'Append-only record of the measure moderation decided; games.blocked_at is the current state.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'action', type: 'text', note: 'block or unblock.' },
    { name: 'reason_code', type: 'text', note: 'The kind the owner is shown (a service restriction, P13§5).' },
    { name: 'measure_uuid', type: 'uuid', note: 'The moderation measure that ordered it.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(game_id)'],
  history: 'It is itself the history (append-only).',
  references: [
    { column: 'game_id', to: 'game.games' },
    { column: 'measure_uuid', to: 'moderation measure (external identifier)' }
  ]
});

PH.table({
  id: 'game.game_categories', domain: 'game', status: 'planned',
  name: 'game_categories',
  summary: 'The one-per-game categories, admin-managed, named in the three languages.',
  purpose: 'Never hardcoded (B: admin manages categories); seeded with the V-14 starter set.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'slug', type: 'text', note: 'Stable key used in public URLs.' },
    { name: 'names', type: 'jsonb', note: 'es, en, pt names.' },
    { name: 'position', type: 'int', note: 'Display order in Discovery.' },
    { name: 'retired_at', type: 'timestamptz', note: 'Set once; a retired category keeps old references but cannot be picked.' },
    { name: 'created_by_user_uuid', type: 'uuid', note: 'Empty for the seeded rows.' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (slug)'],
  history: 'Rows are retired, not deleted; renames append a change row in game_taxonomy_changes.',
  references: [{ column: 'created_by_user_uuid', to: 'identity user' }]
});

PH.table({
  id: 'game.game_tags', domain: 'game', status: 'planned',
  name: 'game_tags',
  summary: 'The allowed descriptive tags, admin-managed (B: admin manages which tags exist and filter).',
  purpose: 'A controlled vocabulary so search by tags stays useful; games pick several.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'slug', type: 'text', note: '' },
    { name: 'names', type: 'jsonb', note: 'es, en, pt names.' },
    { name: 'retired_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_by_user_uuid', type: 'uuid', note: 'Empty for the seeded rows.' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (slug)'],
  history: 'Rows are retired, not deleted; renames append a change row in game_taxonomy_changes.',
  references: [{ column: 'created_by_user_uuid', to: 'identity user' }]
});

PH.table({
  id: 'game.game_taxonomy_changes', domain: 'game', status: 'planned',
  name: 'game_taxonomy_changes',
  summary: 'Every admin change to a category or tag (create, rename, reorder, retire), with actor and reason.',
  purpose: 'Append-only; the admin-configurable tables rule of D-011.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'kind', type: 'text', note: 'category or tag.' },
    { name: 'subject_id', type: 'bigint', note: 'The category or tag row.' },
    { name: 'action', type: 'text', note: 'create, rename, reorder or retire.' },
    { name: 'detail', type: 'jsonb', note: 'What changed.' },
    { name: 'created_by_user_uuid', type: 'uuid', note: '' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(kind, subject_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'created_by_user_uuid', to: 'identity user' }]
});

PH.table({
  id: 'game.game_game_tags', domain: 'game', status: 'planned',
  name: 'game_game_tags',
  summary: 'The working tags of a game, edited by the owner on the card.',
  purpose: 'What the next publication request snapshots; Discovery never reads it.',
  columns: [
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'tag_id', type: 'bigint', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (game_id, tag_id)'],
  history: 'Rows are added and removed; the durable record is the publication snapshot.',
  references: [
    { column: 'game_id', to: 'game.games' },
    { column: 'tag_id', to: 'game.game_tags' }
  ]
});

PH.table({
  id: 'game.game_template_grants', domain: 'game', status: 'planned',
  name: 'game_template_grants',
  summary: 'Every enable or disable of a game as a template; the latest row is the current state.',
  purpose: 'The explicit acceptance P05§1 asks for, recorded with the exact permission-text version and the asset-rights confirmation.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'action', type: 'text', note: 'enable or disable.' },
    { name: 'permission_text_version', type: 'int', note: 'The compliance text version accepted on enable; empty on disable.' },
    { name: 'assets_confirmed', type: 'boolean', note: 'On enable: the creator confirmed every included asset may be reused (P05§5).' },
    { name: 'created_by_user_uuid', type: 'uuid', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(game_id, id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'game_id', to: 'game.games' }, { column: 'created_by_user_uuid', to: 'identity user' }]
});

PH.table({
  id: 'game.game_provenances', domain: 'game', status: 'planned',
  name: 'game_provenances',
  summary: 'Where a game came from: one row per template version it was copied from or combined with (P05§3).',
  purpose: 'Kept automatically, survives the source being disabled or deleted, and feeds the credits on the public card.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: 'The derived game.' },
    { name: 'source_game_uuid', type: 'uuid', note: 'The template game.' },
    { name: 'source_version_uuid', type: 'uuid', note: 'The specific version copied.' },
    { name: 'source_name', type: 'text', note: 'Snapshot for the credit line, in case the source goes away.' },
    { name: 'permission_text_version', type: 'int', note: 'The permission version in force when the copy was made.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(game_id)', '(source_game_uuid)'],
  history: 'Write-once rows.',
  references: [
    { column: 'game_id', to: 'game.games' },
    { column: 'source_game_uuid', to: 'the source game (external identifier)' }
  ]
});

PH.table({
  id: 'game.game_featured_templates', domain: 'game', status: 'planned',
  name: 'game_featured_templates',
  summary: 'The bounded administrative selection of templates the AI always sees first (P05§1, SPEC_MAP §7).',
  purpose: 'Distinct from "available as a template"; rows are removed by marking, with actor and reason.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'position', type: 'int', note: 'Order within the selection.' },
    { name: 'created_by_user_uuid', type: 'uuid', note: '' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'removed_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['partial UNIQUE (game_id) while not removed'],
  history: 'Removed-not-deleted rows are the history.',
  references: [{ column: 'game_id', to: 'game.games' }, { column: 'created_by_user_uuid', to: 'identity user' }]
});

PH.table({
  id: 'game.game_usage_days', domain: 'game', status: 'planned',
  name: 'game_usage_days',
  summary: 'Sessions started per game per day, fed by play through the queue (D-048).',
  purpose: 'Game\'s own copy of the usage signal: popularity ordering and the real card statistics read here, never across domains.',
  columns: [
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'day', type: 'date', note: 'UTC day.' },
    { name: 'sessions_started', type: 'int', note: 'Incremented per delivered event.' }
  ],
  indexes: ['UNIQUE (game_id, day)'],
  history: 'infrastructure (aggregate counters; the facts live in play.session).',
  references: [{ column: 'game_id', to: 'game.games' }]
});

PH.table({
  id: 'game.game_version_choices', domain: 'game', status: 'planned',
  name: 'game_version_choices',
  summary: 'Every change of the version chosen for new Sessions, including making the game unavailable.',
  purpose: 'Append-only history behind games.chosen_version_uuid (P04§5).',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'version_uuid', type: 'uuid', note: 'Empty when the game was made unavailable.' },
    { name: 'created_by_user_uuid', type: 'uuid', note: 'Empty when the platform chose (the finalize default).' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(game_id)'],
  history: 'It is itself the history (append-only).',
  references: [
    { column: 'game_id', to: 'game.games' },
    { column: 'version_uuid', to: 'play.versions version (external identifier)' },
    { column: 'created_by_user_uuid', to: 'identity user' }
  ]
});

PH.operation({
  id: 'game.check-ownership', domain: 'game', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'CheckOwnership',
  summary: 'Authorizes the owner of a game with no further condition — unlike find-editable-version, which needs an open edition.',
  reads: ['game.games'],
  writes: [],
  rules: ['Found only if the game exists, is not deleted and the caller owns it.'],
  sources: []
});

PH.operation({
  id: 'game.choose-version', domain: 'game', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'ChooseVersion / ClearChosenVersion',
  summary: 'Records which EXECUTABLE version new Sessions use — or that the game is unavailable — and recomputes the offering (P04§5).',
  reads: ['game.games', 'game.game_publications'],
  writes: ['game.games', 'game.game_version_choices'],
  rules: [
    'The orchestrator verified the version in play.versions (EXECUTABLE, same game) before this lands; browser values alone never choose (P15§5).',
    'Changing the choice never touches waiting or running Sessions nor their replays (P04§3).',
    'The offering follows the choice: a chosen version with a retained approval is offered without a new review; one without keeps the previously approved combination in Discovery (P04§4-5, D-045).',
    'Idempotent; every change appends its choice row.'
  ],
  sources: ['P04§3-5']
});

PH.operation({
  id: 'game.request-publication', domain: 'game', kind: 'workflow', effect: 'write', status: 'planned',
  name: 'RequestPublication',
  summary: 'Freezes the working card into a new SUBMITTED combination for the review queue (P04§4).',
  reads: ['game.games', 'game.game_game_tags', 'game.game_card_images', 'game.game_publications'],
  writes: ['game.game_publications', 'game.game_publication_status_changes', 'game.game_publication_tags'],
  rules: [
    'Only the owner; the named version must be EXECUTABLE (version facts arrive from the orchestrator, which read them in play.versions).',
    'The private-content declaration is mandatory and must be "no": a game with private data or images cannot be public (B); it stays playable privately.',
    'A category is required; tags come from the working set; name and description from the working card, images by their object keys.',
    'The academic mark is a claim here; the review confirms or removes it.',
    'One SUBMITTED combination per game: a new request supersedes the previous one (and its review task is cancelled through the flow).',
    'Submitting changes nothing public: while the update is reviewed, Discovery keeps offering the previously approved combination (P04§4).',
    'Idempotent per game and key. A blocked or deleted game refuses.'
  ],
  lifecycle: PUBLICATION_LIFECYCLE,
  sources: ['P04§4', 'P14§2', 'B: private content']
});

PH.operation({
  id: 'game.apply-review-decision', domain: 'game', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'ApplyReviewDecision',
  summary: 'Lands moderation\'s approve or deny on the submitted combination and recomputes what Discovery offers.',
  reads: ['game.games', 'game.game_publications'],
  writes: ['game.games', 'game.game_publications', 'game.game_publication_status_changes', 'game.game_histories'],
  rules: [
    'Approve: the combination becomes APPROVED; games.published_at is set on the first one; the snapshot\'s category and the reviewed academic mark land on the game; the offered publication is recomputed (D-045).',
    'Deny: DENIED with the admin\'s note; nothing public changes.',
    'A game deleted, withdrawn or blocked since submission answers moot and the decision lands as CANCELLED: approving never resurrects it.',
    'Idempotent by publication uuid and decision.'
  ],
  sources: ['P04§4', 'P13§6']
});

PH.operation({
  id: 'game.set-offering', domain: 'game', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'SetOffering',
  summary: 'The owner unpublishes the game from Discovery or offers the approved combination again (P04§6).',
  reads: ['game.games', 'game.game_publications'],
  writes: ['game.games', 'game.game_offering_changes'],
  rules: [
    'Unpublish withdraws the offering and blocks new Sessions from the publication; existing Sessions and other people\'s replays are untouched (P04§6); private use continues.',
    'Unpublish never touches the approval record: re-offering the same approved, unchanged combination needs no new review (P04§5 principle, D-046); a blocked game cannot re-offer.',
    'Both directions are idempotent and recorded with actor.'
  ],
  sources: ['P04§5-6']
});

PH.operation({
  id: 'game.delete-game', domain: 'game', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'DeleteGame',
  summary: 'Single-game deletion by its owner: the same deleted_at the account cleanup writes, for one game (D-036).',
  reads: ['game.games'],
  writes: ['game.games'],
  rules: [
    'Sets deleted_at once; existing Sessions keep running and other people\'s replays are untouched (P04§6).',
    'The offering ends with the game; versions keep what replays need through their retention (P04§3).',
    'Template copies already made are independent and unaffected (P05§4).',
    'Idempotent; distinct from unpublish and from an administrative block (P04§6).'
  ],
  sources: ['P04§6', 'P02§11']
});

PH.operation({
  id: 'game.apply-block', domain: 'game', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'ApplyBlock',
  summary: 'Executes the administrative block or unblock moderation decided (P04§6, P13§5).',
  reads: ['game.games'],
  writes: ['game.games', 'game.game_blocks'],
  rules: [
    'Block: out of Discovery, no new Sessions for anyone (owner included), no publication requests, no template copies; existing Sessions are not interrupted (stopping a dangerous Session is an H9 measure).',
    'The reason the owner sees is a kind, worded as a service restriction; the appeal path arrives with H9.',
    'Unblock restores exactly what the block removed; the offering returns only if it was in force.',
    'Idempotent per measure.'
  ],
  sources: ['P04§6', 'P13§5']
});

PH.operation({
  id: 'game.search-discovery', domain: 'game', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'SearchDiscovery',
  summary: 'The Discovery catalog: popular, by category, text search and filters over offered approved combinations only.',
  reads: ['game.games', 'game.game_publications', 'game.game_publication_tags', 'game.game_categories', 'game.game_tags', 'game.game_usage_days', 'game.game_template_grants'],
  writes: [],
  rules: [
    'Only games with an offered publication, not blocked, not withdrawn, not deleted; everything shown is the approved snapshot (P04§4).',
    'Filters: one category, several tags with any/all, language, template only; text search over the snapshot name (trigram index).',
    'Order: popularity (sessions started in the last 7 days, V-13), then recency; every query scoped by the partial Discovery indexes.',
    'Card statistics are real and only shown at or above the V-13 threshold; below it nothing is shown — no invented activity (P14§2).'
  ],
  limits: [{ what: 'Page size / tags per filter / text length', value: '24 / 10 / 2-80 chars (V-13)', source: 'domain' }],
  sources: ['B: search', 'P14§2']
});

PH.operation({
  id: 'game.read-public-card', domain: 'game', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'ReadPublicCard',
  summary: 'One offered game\'s approved snapshot: the public card with its real statistics, credits and template mark.',
  reads: ['game.games', 'game.game_publications', 'game.game_publication_tags', 'game.game_usage_days', 'game.game_template_grants', 'game.game_provenances', 'game.game_categories', 'game.game_tags'],
  writes: [],
  rules: [
    'Answers only the approved offered combination (P04§4); a withdrawn, blocked or deleted game answers not available.',
    'Shows languages, players, devices, academic mark, grade support and the template permission when granted (P14§2).',
    'Credits name the template sources from the provenance rows (P05§3).',
    'Statistics: sessions started in the last 30 days, shown from the V-13 threshold, with definition and period; omitted below it.'
  ],
  sources: ['P14§2', 'P05§3']
});

PH.operation({
  id: 'game.read-taxonomy', domain: 'game', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'ReadTaxonomy',
  summary: 'The live categories and allowed tags with their names, for Discovery and the card pickers.',
  reads: ['game.game_categories', 'game.game_tags'],
  writes: [],
  rules: ['Public read; retired entries are left out. The admin read includes retired ones and the change history.'],
  sources: ['B: admin', 'P14§2']
});

PH.operation({
  id: 'game.admin-taxonomy', domain: 'game', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'AdminTaxonomy',
  summary: 'The admin creates, renames, reorders or retires categories and tags, with actor and reason.',
  reads: ['game.game_categories', 'game.game_tags'],
  writes: ['game.game_categories', 'game.game_tags', 'game.game_taxonomy_changes'],
  rules: [
    'Retiring never deletes: existing games and snapshots keep their references; the entry just cannot be picked again.',
    'Every change appends its game_taxonomy_changes row (D-011 pattern).'
  ],
  sources: ['B: admin']
});

PH.operation({
  id: 'game.manage-template', domain: 'game', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'ManageTemplate',
  summary: 'Enables a game as a template with the explicit, recorded acceptance, or disables it (P05§1,§4).',
  reads: ['game.games', 'game.game_publications', 'game.game_template_grants'],
  writes: ['game.game_template_grants'],
  rules: [
    'Enable requires an offered approved publication, no block, the acceptance of the current template-permission text at its exact version, and the confirmation that every included asset may be reused (P05§5).',
    'Disabling prevents new copies from then on; existing derivations and their permissions survive (P05§4).',
    'Enabling again after a permission-text change records the new version.',
    'Idempotent: enabling what is enabled, or disabling what is disabled, appends nothing.'
  ],
  sources: ['P05§1', 'P05§4-5']
});

PH.operation({
  id: 'game.record-provenance', domain: 'game', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'RecordProvenance',
  summary: 'Appends the provenance row(s) when a template is applied to an existing draft (combining, P05§3).',
  reads: [],
  writes: ['game.game_provenances'],
  rules: ['Idempotent per game and source version; chats, students, grades and private context never carry over (P05§3).'],
  sources: ['P05§3']
});

PH.operation({
  id: 'game.admin-featured-templates', domain: 'game', kind: 'workflow', effect: 'write', status: 'planned',
  name: 'AdminFeaturedTemplates (List / Add / Reorder / Remove)',
  summary: 'The bounded featured-template selection the AI sees first, managed by the admin.',
  reads: ['game.game_featured_templates', 'game.games', 'game.game_template_grants'],
  writes: ['game.game_featured_templates'],
  rules: [
    'Only a game with a live template grant and an offered publication can be featured; losing either hides it from the selection without deleting the row.',
    'Rows are removed by marking, with actor and reason.'
  ],
  sources: ['P05§1', 'SPEC_MAP §7']
});

PH.operation({
  id: 'game.admin-find-games', domain: 'game', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'AdminFindGames',
  summary: 'The admin finds any game by uuid, name or owner and reads its full state (H10 builds the complete panel).',
  reads: ['game.games', 'game.game_publications', 'game.game_blocks', 'game.game_template_grants'],
  writes: [],
  rules: ['Private and deleted games included: admin management reaches everything (B: admin).'],
  sources: ['B: admin']
});

PH.operation({
  id: 'game.record-game-usage', domain: 'game', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'RecordGameUsage',
  summary: 'Lands one session-started event from play into the daily usage counters (D-048).',
  reads: [],
  writes: ['game.game_usage_days'],
  rules: ['Idempotent by the event\'s source key; an unknown game uuid lands nowhere and the task dies for admin.'],
  sources: ['P14§2', 'B: search']
});

PH.operation({
  id: 'game.update-popularity', domain: 'game', kind: 'workflow', effect: 'write', status: 'planned',
  name: 'UpdatePopularity (hourly periodic task on the game queue)',
  summary: 'Recomputes games.popularity_score from the last 7 days of usage (V-13).',
  reads: ['game.game_usage_days', 'game.games'],
  writes: ['game.games'],
  rules: ['Touches only games with usage in the window or a score above zero; the score is derived, so it carries no history.'],
  sources: ['B: search']
});
