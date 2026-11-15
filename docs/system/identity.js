// Domain: identity. Existing tables and operations loaded by T-004; future ones (student credentials,
// email codes, teacher/student modes, admin second factor) by milestone designs.
PH.domain({
  id: 'identity', parent: null, kind: 'traditional', status: 'implemented',
  name: 'Identity',
  summary: 'Who a person is and how they get in: guests, accounts, student and admin credentials, and the general permission to call each operation.',
  responsibilities: [
    'Give every person a stable user identifier that survives upgrades.',
    'Create guests (display name only, tied to the device, expire after 90 days of inactivity).',
    'Create and recover registered accounts: Google and one-time email codes; link methods; no own passwords.',
    'Upgrade a guest to a new account keeping identity and history; never merge into an existing account.',
    'Issue, renew and recognize credentials, including per-mode tokens (normal, teacher, student, admin).',
    'Issue and revoke the individual per-class student credential, and protect the adult account behind student mode.',
    'Hold the single administrator account with its second factor.',
    'Decide the general permission for each operation (account type, role, mode); limit display-name changes per window.',
    'Authenticate external AI clients (OAuth for MCP).'
  ],
  doesNotKnow: [
    'Whether a student belongs to a class or may join an activity (classroom).',
    'What a plan entitles the account to (plan); switching to teacher mode asks plan through orchestration.',
    'Age, country and acceptance evidence (compliance).',
    'Games, sessions and any resource-level scoping (each owning domain).'
  ],
  references: [
    'No foreign domain data: it is the source of the user identifier the rest of the system stores.'
  ],
  sources: ['B: identity', 'P03', 'P02§9-10']
});

var AI_SESSION_LIFECYCLE = {
  states: ['approved', 'revoked'],
  transitions: [
    { from: 'approved', to: 'revoked', on: 'the user revokes the connection, the client is removed, a code is replayed, or a refresh token is replayed' }
  ]
};

var LOGIN_ATTEMPT_LIFECYCLE = {
  states: ['started', 'answered', 'code_issued', 'completed'],
  transitions: [
    { from: 'started', to: 'answered', on: 'the provider calls back, exactly once' },
    { from: 'answered', to: 'code_issued', on: 'the callback resolves to an existing, linked-guest or created user' },
    { from: 'answered', to: 'cancelled, failed or unavailable', on: 'the provider reports that outcome instead' },
    { from: 'code_issued', to: 'completed', on: 'the one-time code is exchanged within 60 s with the matching PKCE verifier' },
    { from: 'code_issued', to: 'rejected', on: 'the verifier does not match; the code is consumed either way' }
  ]
};

PH.table({
  id: 'identity.identity_users', domain: 'identity', status: 'implemented',
  name: 'identity_users',
  summary: 'One row per stable user identifier, guest or account.',
  purpose: 'The identifier every other domain stores to mean "this person".',
  columns: [
    { name: 'id', type: 'bigserial', note: 'Internal key.' },
    { name: 'user_uuid', type: 'uuid', note: 'External identifier.' },
    { name: 'display_name', type: 'text', note: 'Empty for a new account until it sets one.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (user_uuid)'],
  history: 'Display name changes tracked in identity_display_name_changes.',
  references: []
});

PH.table({
  id: 'identity.identity_accounts', domain: 'identity', status: 'implemented',
  name: 'identity_accounts',
  summary: 'Marks a user as a registered account rather than a guest.',
  purpose: 'Presence of the row is the "registered" fact; no other column.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (user_id)'],
  history: 'infrastructure',
  references: [{ column: 'user_id', to: 'identity.identity_users' }]
});

PH.table({
  id: 'identity.identity_sessions', domain: 'identity', status: 'implemented',
  name: 'identity_sessions',
  summary: 'One row per refresh token: a credentials session.',
  purpose: 'Tracks a login from issue to deactivation, with rotation.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'session_uuid', type: 'uuid', note: '' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'refresh_token', type: 'text', note: 'Stored in clear (phr_ prefix).' },
    { name: 'issued_at', type: 'timestamptz', note: '' },
    { name: 'started_at', type: 'timestamptz', note: 'When the session began (login time).' },
    { name: 'expires_at', type: 'timestamptz', note: 'Empty for a guest session: it never expires.' },
    { name: 'deactivated_at', type: 'timestamptz', note: 'Set once, by logout or replacement.' },
    { name: 'replaced_by_id', type: 'bigint', note: 'Set once, on rotation.' }
  ],
  indexes: ['UNIQUE (refresh_token)', 'partial UNIQUE (session_uuid) while not deactivated', '(user_id)'],
  history: 'Write-once: deactivated_at, replaced_by_id.',
  references: [{ column: 'user_id', to: 'identity.identity_users' }]
});

PH.table({
  id: 'identity.identity_guest_requests', domain: 'identity', status: 'implemented',
  name: 'identity_guest_requests',
  summary: 'Idempotency of guest creation: one row per key.',
  purpose: 'A repeated request with the same key returns the first guest instead of creating another.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'display_name', type: 'text', note: '' },
    { name: 'user_uuid', type: 'uuid', note: '' },
    { name: 'session_uuid', type: 'uuid', note: '' },
    { name: 'created_at', type: 'timestamptz', note: 'Reused for 5 minutes (replay window).' }
  ],
  indexes: ['UNIQUE (idempotency_key)'],
  history: 'Write-once: claimed and completed in one transaction.',
  references: [{ column: 'user_uuid', to: 'identity.identity_users' }]
});

PH.table({
  id: 'identity.identity_external_identities', domain: 'identity', status: 'implemented',
  name: 'identity_external_identities',
  summary: 'Links a provider identity (issuer, subject) to a user.',
  purpose: 'Lets a later login with the same provider find the same user.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'issuer', type: 'text', note: 'google or local.' },
    { name: 'subject', type: 'text', note: 'Provider-side subject id.' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'email', type: 'text', note: 'Observed, not verified by Identity itself.' },
    { name: 'name', type: 'text', note: 'Observed.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (issuer, subject)', '(user_id)'],
  history: 'infrastructure',
  references: [{ column: 'user_id', to: 'identity.identity_users' }]
});

PH.table({
  id: 'identity.identity_login_attempts', domain: 'identity', status: 'implemented',
  name: 'identity_login_attempts',
  summary: 'One row per account-login attempt, from the authorization URL to completion.',
  purpose: 'Carries the current state of a login in its own columns; the history of what it went through is the companion events table.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'state', type: 'text', note: 'Anti-CSRF state sent to the provider.' },
    { name: 'provider', type: 'text', note: 'google or local.' },
    { name: 'code_challenge', type: 'text', note: 'PKCE S256 from the frontend.' },
    { name: 'nonce', type: 'text', note: '' },
    { name: 'provider_code_verifier', type: 'text', note: 'PKCE verifier this backend uses against the provider.' },
    { name: 'guest_user_id', type: 'bigint', note: 'Optional guest to link if the login creates a user.' },
    { name: 'answered_at', type: 'timestamptz', note: 'Set once, before talking to the provider.' },
    { name: 'user_id', type: 'bigint', note: 'Set once the callback resolves.' },
    { name: 'login_code', type: 'text', note: 'One-time code, set once.' },
    { name: 'login_code_expires_at', type: 'timestamptz', note: '60 s from issuing.' },
    { name: 'completed_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: '5 min lifetime from here.' }
  ],
  indexes: ['UNIQUE (state)', 'UNIQUE (login_code)'],
  history: 'Change table identity_login_attempt_events.',
  references: [
    { column: 'guest_user_id', to: 'identity.identity_users' },
    { column: 'user_id', to: 'identity.identity_users' }
  ],
  lifecycle: LOGIN_ATTEMPT_LIFECYCLE
});

PH.table({
  id: 'identity.identity_login_attempt_events', domain: 'identity', status: 'implemented',
  name: 'identity_login_attempt_events',
  summary: 'Every event a login attempt went through.',
  purpose: 'Append-only history: started, answered, cancelled, failed, unavailable, code_issued, completed, rejected (with reason).',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'attempt_id', type: 'bigint', note: '' },
    { name: 'event', type: 'text', note: '' },
    { name: 'reason', type: 'text', note: 'Only for rejected.' },
    { name: 'observed_email', type: 'text', note: '' },
    { name: 'observed_name', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(attempt_id)'],
  history: 'It is itself the history (append-only); kept forever.',
  references: [{ column: 'attempt_id', to: 'identity.identity_login_attempts' }]
});

PH.table({
  id: 'identity.identity_display_name_changes', domain: 'identity', status: 'implemented',
  name: 'identity_display_name_changes',
  summary: 'Every display-name change.',
  purpose: 'Append-only history: from_name, to_name.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'from_name', type: 'text', note: '' },
    { name: 'to_name', type: 'text', note: 'Up to 64 runes.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(user_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'user_id', to: 'identity.identity_users' }]
});

PH.table({
  id: 'identity.identity_resource_links', domain: 'identity', status: 'implemented',
  name: 'identity_resource_links',
  summary: 'A secret link to an opaque resource (kind + id), e.g. a game\'s external-AI link.',
  purpose: 'Identity never learns what the resource is; it only mints and resolves the token.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'user_id', type: 'bigint', note: 'Owner of the link.' },
    { name: 'resource_kind', type: 'text', note: 'Caller-chosen tag, e.g. "game".' },
    { name: 'resource_id', type: 'text', note: 'Caller-chosen opaque id.' },
    { name: 'token_sha256', type: 'text', note: 'The phl_ token itself is never stored.' },
    { name: 'expires_at', type: 'timestamptz', note: '7 days from creation.' },
    { name: 'revoked_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (token_sha256)', 'partial UNIQUE (user_id, resource_kind, resource_id) while not revoked'],
  history: 'Write-once: revoked_at.',
  references: [{ column: 'user_id', to: 'identity.identity_users' }]
});

PH.table({
  id: 'identity.identity_resource_grants', domain: 'identity', status: 'implemented',
  name: 'identity_resource_grants',
  summary: 'A single-use grant over an opaque resource, e.g. an asset upload URL handed to an AI.',
  purpose: 'Short-lived, claimed once; expired-and-unused rows are swept when the same user creates another.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'uuid', type: 'uuid', note: 'External identifier (phg_ token).' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'resource_kind', type: 'text', note: '' },
    { name: 'resource_id', type: 'text', note: '' },
    { name: 'scope', type: 'text', note: 'Caller-chosen, up to 256 bytes.' },
    { name: 'token_sha256', type: 'text', note: '' },
    { name: 'expires_at', type: 'timestamptz', note: '15 min from creation.' },
    { name: 'used_at', type: 'timestamptz', note: 'Set once, on claim.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (uuid)', 'UNIQUE (token_sha256)', '(user_id, expires_at)'],
  history: 'Write-once: used_at. Expired, unused rows older than 1 day are deleted, not kept.',
  references: [{ column: 'user_id', to: 'identity.identity_users' }]
});

PH.table({
  id: 'identity.identity_ai_clients', domain: 'identity', status: 'implemented',
  name: 'identity_ai_clients',
  summary: 'An AI client registered by dynamic client registration (DCR).',
  purpose: 'Public OAuth clients only; up to 8 redirect URIs, name up to 100 runes.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'client_id', type: 'text', note: '' },
    { name: 'name', type: 'text', note: '' },
    { name: 'redirect_uris', type: 'jsonb', note: '' },
    { name: 'grant_types', type: 'jsonb', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (client_id)'],
  history: 'infrastructure',
  references: []
});

PH.table({
  id: 'identity.identity_ai_authorization_requests', domain: 'identity', status: 'implemented',
  name: 'identity_ai_authorization_requests',
  summary: 'An OAuth authorization request awaiting the user\'s consent.',
  purpose: 'PKCE S256 challenge plus the user\'s approve/deny decision and the one-time code it issues.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'request_id', type: 'text', note: '' },
    { name: 'client_id', type: 'text', note: '' },
    { name: 'code_challenge', type: 'text', note: '' },
    { name: 'scope', type: 'text', note: 'Only "games" exists today.' },
    { name: 'resource', type: 'text', note: 'MCP resource audience.' },
    { name: 'user_id', type: 'bigint', note: 'Set when the user decides.' },
    { name: 'decided_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'approved', type: 'boolean', note: '' },
    { name: 'code_sha256', type: 'text', note: 'Set once, if approved.' },
    { name: 'code_expires_at', type: 'timestamptz', note: '2 min from issuing.' },
    { name: 'code_used_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: '10 min lifetime from here.' }
  ],
  indexes: ['UNIQUE (request_id)', 'UNIQUE (code_sha256)'],
  history: 'Write-once: decided_at, code_used_at.',
  references: [{ column: 'user_id', to: 'identity.identity_users' }]
});

PH.table({
  id: 'identity.identity_ai_sessions', domain: 'identity', status: 'implemented',
  name: 'identity_ai_sessions',
  summary: 'One approved AI connection per (user, client).',
  purpose: 'Live until the user or the client revokes it, or token reuse is detected.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'connection_id', type: 'uuid', note: '' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'client_id', type: 'text', note: '' },
    { name: 'revoked_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'revoke_reason', type: 'text', note: 'user, client, code_reuse or refresh_reuse.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (connection_id)', '(user_id, created_at)'],
  history: 'Write-once: revoked_at, revoke_reason.',
  references: [{ column: 'user_id', to: 'identity.identity_users' }],
  lifecycle: AI_SESSION_LIFECYCLE
});

PH.table({
  id: 'identity.identity_ai_refresh_tokens', domain: 'identity', status: 'implemented',
  name: 'identity_ai_refresh_tokens',
  summary: 'Rotating refresh tokens for an AI connection.',
  purpose: 'Each refresh consumes one and issues the next; reuse of a used token revokes the connection.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'session_row_id', type: 'bigint', note: 'The AI session it belongs to.' },
    { name: 'token_sha256', type: 'text', note: '' },
    { name: 'expires_at', type: 'timestamptz', note: '30 days.' },
    { name: 'used_at', type: 'timestamptz', note: 'Set once, on rotation.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (token_sha256)', '(session_row_id, created_at)'],
  history: 'Write-once: used_at.',
  references: [{ column: 'session_row_id', to: 'identity.identity_ai_sessions' }]
});

PH.operation({
  id: 'identity.authorize', domain: 'identity', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'Authorize',
  summary: 'Verifies an access JWT and checks it against the operation catalog, without reading the database.',
  reads: [],
  writes: [],
  rules: [
    'The token carries user uuid, display name and level; it is not looked up.',
    'An operation absent from the catalog is forbidden and raises an alert.',
    'Outcomes: allowed, unauthenticated or forbidden.'
  ],
  sources: []
});

PH.operation({
  id: 'identity.list-operations', domain: 'identity', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'ListOperations',
  summary: 'Publishes the operation-to-level catalog.',
  reads: [], writes: [], rules: [], sources: []
});

PH.operation({
  id: 'identity.create-guest', domain: 'identity', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'CreateGuest',
  summary: 'Creates a guest with a normalized display name and a credentials session.',
  reads: ['identity.identity_guest_requests'],
  writes: ['identity.identity_guest_requests', 'identity.identity_users', 'identity.identity_sessions'],
  rules: ['Idempotent by key; the same key replays its first outcome for 5 minutes.'],
  limits: [{ what: 'Guests per client IP', value: '200/hour', source: 'domain' }],
  sources: []
});

PH.operation({
  id: 'identity.refresh', domain: 'identity', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'Refresh',
  summary: 'Issues a new access token and rotates the refresh token when it is old enough.',
  reads: ['identity.identity_sessions'],
  writes: ['identity.identity_sessions'],
  rules: [
    'Rotates if the current refresh token is more than 5 h old.',
    'The immediate predecessor is still accepted for 5 min after rotation (replaced_by_id).',
    'Otherwise refresh_invalid.'
  ],
  limits: [
    { what: 'Account refresh lifetime', value: '30 d without use / 90 d from login; guests never expire', source: 'domain' },
    { what: 'Rotation / predecessor grace', value: '5 h / 5 min', source: 'domain' }
  ],
  sources: []
});

PH.operation({
  id: 'identity.end-session', domain: 'identity', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'EndSession',
  summary: 'Logout: deactivates the session behind a refresh token, idempotently.',
  reads: ['identity.identity_sessions'], writes: ['identity.identity_sessions'], rules: [], sources: []
});

PH.operation({
  id: 'identity.set-display-name', domain: 'identity', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'SetDisplayName',
  summary: 'Sets the display name and records the change.',
  reads: ['identity.identity_users'],
  writes: ['identity.identity_users', 'identity.identity_display_name_changes'],
  rules: ['Up to 64 runes.'],
  sources: []
});

PH.operation({
  id: 'identity.account-login', domain: 'identity', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'accountlogin (StartLogin / HandleCallback / CompleteLogin)',
  summary: 'Signs a person in or registers them through a provider, with PKCE and a one-time code to the frontend.',
  reads: ['identity.identity_login_attempts', 'identity.identity_external_identities'],
  writes: [
    'identity.identity_login_attempts', 'identity.identity_login_attempt_events',
    'identity.identity_users', 'identity.identity_external_identities', 'identity.identity_sessions'
  ],
  rules: [
    'The attempt is marked answered exactly once, before talking to the provider.',
    '(issuer, subject) resolves to an existing user, a linked guest (inherits the guest\'s uuid), or a newly created user.',
    'Completing validates the PKCE verifier; the code is consumed either way.',
    'Completing a login ends the replaced guest\'s session in the same transaction.'
  ],
  limits: [
    { what: 'Login attempt / one-time code lifetime', value: '5 min / 60 s', source: 'domain' }
  ],
  steps: [
    'StartLogin: record the attempt and return the provider authorization URL.',
    'HandleCallback: mark it answered once, exchange the code with the provider, resolve the user, issue a one-time code.',
    'CompleteLogin: validate the verifier, open the account session, end the replaced guest session.'
  ],
  lifecycle: LOGIN_ATTEMPT_LIFECYCLE,
  sources: []
});

PH.operation({
  id: 'identity.resource-link', domain: 'identity', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'CreateResourceLink / RevokeResourceLink',
  summary: 'Secret link to an opaque resource; one open link per user and resource.',
  reads: ['identity.identity_resource_links'],
  writes: ['identity.identity_resource_links'],
  rules: ['Only the SHA-256 of the token is stored.'],
  limits: [{ what: 'Resource link lifetime', value: '7 days', source: 'domain' }],
  sources: []
});

PH.operation({
  id: 'identity.resource-link-status', domain: 'identity', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'ResourceLinkStatus',
  summary: 'Says whether a live link exists for a resource; never returns the link itself.',
  reads: ['identity.identity_resource_links'], writes: [], rules: [], sources: []
});

PH.operation({
  id: 'identity.resolve-resource-link', domain: 'identity', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'ResolveResourceLink',
  summary: 'Token to user, kind and resource id; outcomes valid, unknown, expired, revoked.',
  reads: ['identity.identity_resource_links'], writes: [], rules: [], sources: []
});

PH.operation({
  id: 'identity.resource-grant', domain: 'identity', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'CreateResourceGrant / ClaimResourceGrant / ReleaseResourceGrant',
  summary: 'Single-use grant over an opaque resource; claim marks it used, release discards it.',
  reads: ['identity.identity_resource_grants'],
  writes: ['identity.identity_resource_grants'],
  rules: ['At most 20 live grants per user (ErrTooManyResourceGrants); older expired-and-unused grants of the same user are swept on create.'],
  limits: [{ what: 'Resource grant lifetime / scope size', value: '15 min / 256 bytes', source: 'domain' }],
  sources: []
});

PH.operation({
  id: 'identity.ai-oauth', domain: 'identity', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'RegisterClient / StartAuthorization / DecideAuthorization / ExchangeToken / Revoke / RevokeConnection',
  summary: 'OAuth 2.1 authorization server for AI clients: dynamic registration, PKCE authorize+consent, token exchange and rotation, RFC 7009 revocation.',
  reads: ['identity.identity_ai_clients', 'identity.identity_ai_authorization_requests', 'identity.identity_ai_sessions', 'identity.identity_ai_refresh_tokens'],
  writes: ['identity.identity_ai_clients', 'identity.identity_ai_authorization_requests', 'identity.identity_ai_sessions', 'identity.identity_ai_refresh_tokens'],
  rules: [
    'Single scope "games": read, create and change games; never publish.',
    'Reusing a consumed authorization code or a used refresh token revokes the connection (code_reuse / refresh_reuse).'
  ],
  limits: [
    { what: 'AI access token / refresh / request / code lifetime', value: '1 h / 30 d / 10 min / 2 min', source: 'domain' },
    { what: 'Redirect URIs / client name', value: '8 max / 100 runes', source: 'domain' }
  ],
  lifecycle: AI_SESSION_LIFECYCLE,
  sources: []
});

PH.operation({
  id: 'identity.ai-oauth-reads', domain: 'identity', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'ReadAuthorization / VerifyAccessToken / ListConnections',
  summary: 'Reads for the AI OAuth server: a pending consent request, token verification on every MCP call, and the user\'s connections.',
  reads: ['identity.identity_ai_authorization_requests', 'identity.identity_ai_sessions', 'identity.identity_ai_refresh_tokens'],
  writes: [], rules: [], sources: []
});
