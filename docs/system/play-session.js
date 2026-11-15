// Subdomain: play.session. Durable state of Sessions and game versions.
// Only the create-session path is detailed here; the rest is loaded by a later task.
PH.domain({
  id: 'play.session', parent: 'play', kind: 'traditional', status: 'implemented',
  name: 'Play session',
  summary: 'A Session and its lifecycle: lobby, participants, start, closing reasons, original results and history.',
  responsibilities: [
    'Create Sessions, start them and end them; it is the only path to an end state (completion, host cancellation, expiration, abandonment, technical interruption).',
    'Issue and release join codes and links; open and close admissions.',
    'Keep the executable version each Session is pinned to.',
    'Know participants: who joined, left, was kicked (a kicked identity does not return) or disconnected, with the 60 s seat margin.',
    'Enforce the duration limits (lobby 30 min, inactivity 10 min, room 4 h).',
    'Keep the original result per participant delivered by the game, untouched.',
    'Answer the history of played and hosted Sessions.',
    'Repeat a closed Session as a new lobby with the same game, version and configuration.'
  ],
  doesNotKnow: [
    'Who may play a game (game decides).',
    'Live execution (play.runtime).',
    'Whether a Session belongs to a class or what a result means academically (classroom).'
  ],
  sources: ['B: session', 'P04§3', 'P06', 'P08§2', 'P15§2']
});

var SESSION_LIFECYCLE = {
  states: ['LOBBY', 'RUNNING', 'FINISHED', 'CANCELLED', 'EXPIRED', 'ABORTED'],
  transitions: [
    { from: 'LOBBY', to: 'RUNNING', on: 'host starts the game' },
    { from: 'RUNNING', to: 'FINISHED', on: 'GAME_FINISHED' },
    { from: 'LOBBY, RUNNING', to: 'CANCELLED', on: 'HOST_CANCELLED or PLATFORM_TERMINATED' },
    { from: 'RUNNING', to: 'CANCELLED', on: 'ALL_PLAYERS_LEFT' },
    { from: 'LOBBY', to: 'EXPIRED', on: 'LOBBY_EXPIRED (30 min)' },
    { from: 'RUNNING', to: 'EXPIRED', on: 'INACTIVITY_TIMEOUT (10 min)' },
    { from: 'LOBBY, RUNNING', to: 'EXPIRED', on: 'MAX_LIFETIME (4 h)' },
    { from: 'LOBBY', to: 'ABORTED', on: 'GAME_START_FAILURE' },
    { from: 'RUNNING', to: 'ABORTED', on: 'SCRIPT_FAILURE, UNRECOVERABLE_INFRA_FAILURE or OWNER_LOST' }
  ]
};

PH.table({
  id: 'play.session.play_sessions', domain: 'play.session', status: 'implemented',
  name: 'play_sessions',
  summary: 'One row per Session, pinned to one executable version.',
  purpose: 'A Session from lobby to its end: who created it, its status and its deadlines.',
  columns: [
    { name: 'id', type: 'bigserial', note: 'Internal key.' },
    { name: 'uuid', type: 'uuid', note: 'External identifier.' },
    { name: 'game_version_id', type: 'bigint', note: 'The version it is pinned to.' },
    { name: 'created_by_user_uuid', type: 'uuid', note: 'The host (Identity user).' },
    { name: 'status', type: 'text', note: 'See lifecycle.' },
    { name: 'end_cause', type: 'text', note: 'Why it ended; empty while live.' },
    { name: 'lobby_expires_at', type: 'timestamptz', note: 'Lobby deadline (30 min).' },
    { name: 'expires_at', type: 'timestamptz', note: 'Maximum lifetime (4 h).' },
    { name: 'owner_lease_expires_at', type: 'timestamptz', note: 'Live owner lease; overwritten without history.' },
    { name: 'created_at', type: 'timestamptz', note: '' },
    { name: 'updated_at', type: 'timestamptz', note: '' }
  ],
  indexes: [
    'UNIQUE (uuid)',
    '(game_version_id)',
    '(lobby_expires_at) only for LOBBY',
    '(expires_at) only for RUNNING',
    '(owner_lease_expires_at) only for RUNNING'
  ],
  history: 'Change table play_session_status_changes, written in the same transaction.',
  references: [
    { column: 'game_version_id', to: 'play.session.play_game_versions' },
    { column: 'created_by_user_uuid', to: 'identity user' }
  ],
  lifecycle: SESSION_LIFECYCLE
});

PH.table({
  id: 'play.session.play_session_status_changes', domain: 'play.session', status: 'implemented',
  name: 'play_session_status_changes',
  summary: 'Every status a Session went through.',
  purpose: 'Append-only history of Session status; the first row is the creation.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'session_id', type: 'bigint', note: '' },
    { name: 'from_status', type: 'text', note: 'Empty for the creation.' },
    { name: 'to_status', type: 'text', note: '' },
    { name: 'end_cause', type: 'text', note: '' },
    { name: 'changed_by_user_uuid', type: 'uuid', note: 'Empty when the platform made the change.' },
    { name: 'changed_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(session_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'session_id', to: 'play.session.play_sessions' }]
});

PH.table({
  id: 'play.session.play_session_join_codes', domain: 'play.session', status: 'implemented',
  name: 'play_session_join_codes',
  summary: 'The 6-character code players type to join a lobby.',
  purpose: 'A code is active until released; only one active row may hold a given code, so codes are reused.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'session_id', type: 'bigint', note: '' },
    { name: 'code', type: 'text', note: '6 characters, no 0/O/1/I/L.' },
    { name: 'expires_at', type: 'timestamptz', note: 'Same as the lobby deadline.' },
    { name: 'released_at', type: 'timestamptz', note: 'Set once, when the game starts or ends.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(session_id)', 'UNIQUE (code) while not released', '(code, id)'],
  history: 'Write-once columns: created_at, released_at.',
  references: [{ column: 'session_id', to: 'play.session.play_sessions' }]
});

PH.table({
  id: 'play.session.play_create_session_requests', domain: 'play.session', status: 'implemented',
  name: 'play_create_session_requests',
  summary: 'Idempotency of Session creation: one row per user and key.',
  purpose: 'A repeated request with the same key returns the first outcome instead of creating another Session.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'user_uuid', type: 'uuid', note: '' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'game_uuid', type: 'uuid', note: 'Same key with another game = conflict.' },
    { name: 'outcome', type: 'text', note: 'Set in the same transaction.' },
    { name: 'session_id', type: 'bigint', note: 'The Session created, if any.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (user_uuid, idempotency_key)', '(session_id)'],
  history: 'Write-once: claimed and completed in one transaction.',
  references: [
    { column: 'session_id', to: 'play.session.play_sessions' },
    { column: 'game_uuid', to: 'game game' },
    { column: 'user_uuid', to: 'identity user' }
  ]
});

PH.table({
  id: 'play.session.play_games', domain: 'play.session', status: 'implemented',
  name: 'play_games',
  summary: 'A game as Play sees it: its identifier and the version played today.',
  purpose: 'Points to the executable version new Sessions use.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_uuid', type: 'uuid', note: 'External identifier from Game.' },
    { name: 'current_version_id', type: 'bigint', note: 'Version used by new Sessions.' },
    { name: 'created_at', type: 'timestamptz', note: '' },
    { name: 'updated_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (game_uuid)', '(current_version_id)'],
  history: 'Detailed by a later task.',
  references: [
    { column: 'game_uuid', to: 'game game' },
    { column: 'current_version_id', to: 'play.session.play_game_versions' }
  ]
});

PH.table({
  id: 'play.session.play_game_versions', domain: 'play.session', status: 'implemented',
  name: 'play_game_versions',
  summary: 'The versions of a game; only an EXECUTABLE one can be played.',
  purpose: 'Content locators and the executable condition of each version.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'uuid', type: 'uuid', note: 'Same as the Game edition.' },
    { name: 'game_id', type: 'bigint', note: '' },
    { name: 'status', type: 'text', note: 'DRAFT or EXECUTABLE.' },
    { name: 'manifest', type: 'jsonb', note: 'Structural facts the runtime needs.' },
    { name: 'script_object_key, script_sha256, script_size', type: 'text, text, bigint', note: 'Backend script in object storage.' },
    { name: 'script_profile', type: 'text', note: 'Rules the script is checked against.' },
    { name: 'frontend_object_key, frontend_sha256, frontend_size', type: 'text, text, bigint', note: 'Frontend package in object storage.' },
    { name: 'aspect_ratio', type: 'text', note: '' },
    { name: 'languages', type: 'jsonb', note: '' },
    { name: 'current_revision', type: 'bigint', note: 'Draft revision.' },
    { name: 'executable_at', type: 'timestamptz', note: 'Set once, when frozen.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (uuid)', '(game_id)', 'UNIQUE (game_id) only for DRAFT', '(script_sha256)', '(frontend_sha256)'],
  history: 'Detailed by a later task.',
  references: [{ column: 'game_id', to: 'play.session.play_games' }],
  lifecycle: { states: ['DRAFT', 'EXECUTABLE'], transitions: [{ from: 'DRAFT', to: 'EXECUTABLE', on: 'publication freezes the version' }] }
});

PH.operation({
  id: 'play.session.create-session', domain: 'play.session', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'CreateSession',
  summary: 'Creates a Session in lobby on the game\'s current executable version, with a join code.',
  reads: ['play.session.play_games', 'play.session.play_game_versions', 'play.session.play_sessions'],
  writes: [
    'play.session.play_create_session_requests',
    'play.session.play_sessions',
    'play.session.play_session_status_changes',
    'play.session.play_session_join_codes'
  ],
  rules: [
    'Everything happens in one transaction.',
    'The same user and key return the first outcome; the same key with another game is a conflict.',
    'A creator has at most the allowed number of live Sessions (lobby or running); otherwise active_session_limit.',
    'A game without a current version is game_unavailable.',
    'The Session is pinned to that version for its whole life.',
    'The join code has 6 characters from an alphabet without 0, O, 1, I, L.'
  ],
  limits: [
    { what: 'Live Sessions per creator', value: '1 by default (environment setting)', source: 'plan' },
    { what: 'Lobby duration and join code life', value: '30 min', source: 'domain' },
    { what: 'Maximum Session lifetime', value: '4 h', source: 'domain' },
    { what: 'Join code generation attempts', value: '10', source: 'domain' }
  ],
  steps: [
    'Claim the request (user, key).',
    'Lock the creator and count their live Sessions.',
    'Find the game\'s current executable version.',
    'Create the LOBBY Session and record its first status.',
    'Assign a free join code.',
    'Complete the request with the outcome.'
  ],
  sources: ['P04§3', 'P06']
});
