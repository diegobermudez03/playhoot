// Subdomain: play.session. Durable state of Sessions and game versions.
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
  id: 'play.session.play_session_participants', domain: 'play.session', status: 'implemented',
  name: 'play_session_participants',
  summary: 'The roster assigned when a Session starts.',
  purpose: 'Participant ids (P1..Pn) are fixed at start; this is not live presence.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'session_id', type: 'bigint', note: '' },
    { name: 'user_uuid', type: 'uuid', note: '' },
    { name: 'participant_id', type: 'text', note: 'P1, P2, ...' },
    { name: 'display_name', type: 'text', note: 'Snapshot at start.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (session_id, participant_id)', 'UNIQUE (session_id, user_uuid)'],
  history: 'infrastructure',
  references: [
    { column: 'session_id', to: 'play.session.play_sessions' },
    { column: 'user_uuid', to: 'identity user' }
  ]
});

PH.table({
  id: 'play.session.play_session_checkpoints', domain: 'play.session', status: 'implemented',
  name: 'play_session_checkpoints',
  summary: 'The latest recoverable state of a running Session.',
  purpose: 'Overwritten in place every ≥5 s while live; lets a Session recover after a platform failure.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'session_id', type: 'bigint', note: '' },
    { name: 'content', type: 'bytea', note: 'Opaque game state.' },
    { name: 'context_revision', type: 'bigint', note: '' },
    { name: 'updated_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (session_id)'],
  history: 'Overwritten without history, by design: high-frequency live data belongs to checkpoints and replay, not rows.',
  references: [{ column: 'session_id', to: 'play.session.play_sessions' }]
});

PH.table({
  id: 'play.session.play_game_fault_reports', domain: 'play.session', status: 'implemented',
  name: 'play_game_fault_reports',
  summary: 'An audit trail of technical game faults, client- or server-reported.',
  purpose: 'Insert-only; it is read by nobody today, it only audits.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'session_id', type: 'bigint', note: '' },
    { name: 'source', type: 'text', note: 'CLIENT or SERVER.' },
    { name: 'kind', type: 'text', note: 'ARTIFACT_UNREACHABLE, ARTIFACT_HASH_MISMATCH, READY_TIMEOUT, UNSUPPORTED_BRIDGE_VERSION, CONTAINER_VIOLATION, BRIDGE_ABUSE, INVALID_SAMPLE, INVALID_COMMAND or SERVER_LIMIT.' },
    { name: 'detail', type: 'jsonb', note: '' },
    { name: 'occurrences', type: 'int', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(session_id)'],
  history: 'Insert-only audit log.',
  references: [{ column: 'session_id', to: 'play.session.play_sessions' }]
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

PH.operation({
  id: 'play.session.commit-start', domain: 'play.session', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'CheckStartEligibility / CommitStart',
  summary: 'Starts a lobby: assigns the roster and the first checkpoint, moves it to RUNNING, releases unused join codes.',
  reads: ['play.session.play_sessions', 'play.session.play_session_join_codes'],
  writes: [
    'play.session.play_sessions', 'play.session.play_session_status_changes',
    'play.session.play_session_participants', 'play.session.play_session_checkpoints', 'play.session.play_session_join_codes'
  ],
  rules: ['One transaction; only from LOBBY.'],
  sources: []
});

PH.operation({
  id: 'play.session.end-session', domain: 'play.session', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'EndSession',
  summary: 'The only path to a terminal Session state.',
  reads: ['play.session.play_sessions'],
  writes: ['play.session.play_sessions', 'play.session.play_session_status_changes'],
  rules: ['Cause determines the resulting terminal state and which starting states accept it (see lifecycle).'],
  lifecycle: SESSION_LIFECYCLE,
  sources: []
});

PH.operation({
  id: 'play.session.load-running-session', domain: 'play.session', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'LoadRunningSession',
  summary: 'Loads a RUNNING Session\'s pinned version and roster for a reattaching controller.',
  reads: ['play.session.play_sessions', 'play.session.play_session_participants', 'play.session.play_session_checkpoints'],
  writes: [], rules: [], sources: []
});

PH.operation({
  id: 'play.session.record-owner-heartbeat', domain: 'play.session', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'RecordOwnerHeartbeat',
  summary: 'Renews the live owner lease of a running Session.',
  reads: [], writes: ['play.session.play_sessions'],
  rules: ['Overwrites owner_lease_expires_at without history.'],
  limits: [{ what: 'Owner lease / heartbeat', value: '10 min / every 30 s', source: 'domain' }],
  sources: []
});

PH.operation({
  id: 'play.session.save-checkpoint', domain: 'play.session', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'SaveCheckpoint',
  summary: 'Overwrites the Session\'s recoverable state.',
  reads: [], writes: ['play.session.play_session_checkpoints'],
  rules: ['At most every 5 s.'],
  sources: []
});

PH.operation({
  id: 'play.session.check-join', domain: 'play.session', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'checkadminjoin / checkplayerjoin / checklivesession / resolvejoincode',
  summary: 'Eligibility checks and join-code lookup before admitting a WebSocket connection.',
  reads: ['play.session.play_sessions', 'play.session.play_session_join_codes'],
  writes: [],
  rules: [
    'Admin: RUNNING, or current LOBBY and the caller is the creator.',
    'Player: current LOBBY; RUNNING answers session_started instead.',
    'CheckLiveSession: live, not_found, ended or lobby_expired.',
    'ResolveJoinCode is case-insensitive: found, expired, session_started or invalid.'
  ],
  sources: []
});

PH.operation({
  id: 'play.session.frontend-access', domain: 'play.session', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'FrontendAccess',
  summary: 'Signed URLs to a Session\'s frontend package and assets.',
  reads: ['play.versions.play_game_version_signed_urls'], writes: [],
  rules: ['One signed batch per version, shared across Sessions and instances; 24 h, reused until within 5 min of expiry.'],
  sources: []
});

PH.operation({
  id: 'play.session.load-runtime-manifest', domain: 'play.session', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'LoadRuntimeManifest / LoadSessionLimits',
  summary: 'The version\'s manifest and input requirements, and a Session\'s deadlines.',
  reads: ['play.session.play_game_versions', 'play.versions.play_game_version_input_requirements', 'play.session.play_sessions'],
  writes: [], rules: [], sources: []
});

PH.operation({
  id: 'play.session.record-game-fault', domain: 'play.session', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'recordgamefault',
  summary: 'Records one game_fault reported by a participant.',
  reads: [], writes: ['play.session.play_game_fault_reports'],
  rules: ['Only while RUNNING, only from a participant.'],
  sources: []
});

PH.operation({
  id: 'play.session.record-limit-hits', domain: 'play.session', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'recordlimithits',
  summary: 'Flushes live limit violations (SERVER_LIMIT) into the fault-report audit.',
  reads: [], writes: ['play.session.play_game_fault_reports'],
  rules: ['Every 5 s.'],
  sources: []
});

PH.operation({
  id: 'play.session.keep-simulation-result', domain: 'play.session', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'KeepSimulationResult / FindSimulationResult / CountSimulations',
  summary: 'Stores and reads a simulation result produced by play.runtime.',
  reads: ['play.versions.play_simulation_results'], writes: ['play.versions.play_simulation_results'],
  rules: ['No cap applied on how many simulations a version may run.'],
  sources: []
});

PH.operation({
  id: 'play.session.read-sdk-guide', domain: 'play.session', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'readsdkguide',
  summary: 'Reads one page of the SDK guide from the toolchain checkout.',
  reads: [], writes: [],
  rules: ['Up to 256 KiB per page; unavailable if the toolchain checkout is not configured.'],
  sources: []
});

PH.operation({
  id: 'play.session.reconcile-sessions', domain: 'play.session', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'reconcilesessions (batch, every 5 min on every instance)',
  summary: 'Closes Sessions that have silently overrun their deadlines.',
  reads: ['play.session.play_sessions'],
  writes: ['play.session.play_sessions', 'play.session.play_session_status_changes'],
  rules: [
    'Expired lobbies end LOBBY_EXPIRED.',
    'RUNNING past expires_at ends MAX_LIFETIME, or GAME_FINISHED if the replay already finalized.',
    'RUNNING with an expired owner lease ends OWNER_LOST.',
    'Processes in pages of 100 with a 5 s lock timeout.'
  ],
  sources: []
});
