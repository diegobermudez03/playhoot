// Domain: api (entry). Endpoints live here.
PH.domain({
  id: 'api', parent: null, kind: 'entry', status: 'implemented',
  name: 'API',
  summary: 'Entry point from outside: HTTP, WebSocket and MCP routes that hand each request to one flow or operation.',
  responsibilities: [
    'Authenticate the caller and check the operation level before anything else.',
    'Limit requests per user and per client address (network limits).',
    'Call one orchestrator flow, composer read, or single-domain operation per request.',
    'Map WebSocket and MCP connections to logical users; domains only see their own realtime interface.',
    'Report metrics at the network level only (path, time, response code).'
  ],
  doesNotKnow: ['Business rules of any domain.', 'What an HTTP result means for the business (each domain has its own outcome metric).'],
  sources: ['B: api']
});

var DEFAULT_LIMIT = 'Default: 5/s and 30/min per user; 20/s and 300/min per client address.';
var AI_LINK_LIMIT = '20/s and 300/min per owner (AI-link access).';
var MCP_LIMIT = '20/s and 300/min per user.';

// ---------- play / sessions ----------
PH.endpoint({
  id: 'POST:/v1/sessions', transport: 'http', access: 'account', status: 'implemented',
  name: 'Create session',
  summary: 'Creates a Session in lobby for a game the caller may play.',
  invokes: 'create-session',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'WS:/v1/play/join/sessions/{sessionUUID}/admin', transport: 'ws', access: 'account', status: 'implemented',
  name: 'Join session as admin',
  summary: 'Live connection that drives a Session as its host (lobby control, start, perspectives).',
  invokes: 'play.runtime.live-connection',
  networkLimit: '120 msg/s and 6000/min per connection; select_perspective 5/s, start/cancel 2/s.'
});

PH.endpoint({
  id: 'WS:/v1/play/join/sessions/{sessionUUID}/player', transport: 'ws', access: 'guest', status: 'implemented',
  name: 'Join session as player',
  summary: 'Live connection as a participant; ?device=desktop|mobile.',
  invokes: 'play.runtime.live-connection',
  networkLimit: '120 msg/s and 6000/min per connection; sample 60/s (queue 64), command 20/s.'
});

PH.endpoint({
  id: 'WS:/v1/play/join/{joinCode}', transport: 'ws', access: 'guest', status: 'implemented',
  name: 'Join session by code',
  summary: 'Resolves a 6-character join code, then connects as a player the same way as the direct player route.',
  invokes: 'play.runtime.live-connection',
  networkLimit: '120 msg/s and 6000/min per connection.'
});

PH.endpoint({
  id: 'WS:/v1/play/previews/{previewUUID}/connect', transport: 'ws', access: 'account', status: 'implemented',
  name: 'Connect to preview',
  summary: 'Live connection to a draft preview, this instance only (no owner routing).',
  invokes: 'play.runtime.previews',
  networkLimit: 'Preview control 10/s.'
});

// ---------- identity ----------
PH.endpoint({
  id: 'POST:/v1/guests', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Create guest',
  summary: 'Creates a guest identity with a display name and credentials.',
  invokes: 'identity.create-guest',
  networkLimit: '200/h per client IP (plus default).'
});

PH.endpoint({
  id: 'GET:/v1/identity/operations', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'List operations',
  summary: 'Publishes the operation-to-level catalog.',
  invokes: 'identity.list-operations',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'POST:/v1/identity/refresh', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Refresh credentials',
  summary: 'Renews the access token, rotating the refresh token when it is old enough.',
  invokes: 'identity.refresh',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'POST:/v1/identity/logout', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Logout',
  summary: 'Deactivates the session behind a refresh token.',
  invokes: 'identity.end-session',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'POST:/v1/identity/login/start', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Start account login',
  summary: 'Starts a provider login; an optional guest token links that guest if the login creates a user.',
  invokes: 'identity.account-login',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'GET:/v1/identity/login/{provider}/callback', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Login callback',
  summary: 'The provider calls this back; redirects the browser to the web sign-in completion page.',
  invokes: 'identity.account-login',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'POST:/v1/identity/login/complete', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Complete account login',
  summary: 'Exchanges the one-time code and PKCE verifier for a credentials session.',
  invokes: 'identity.account-login',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'GET:/v1/identity/login/providers', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'List login providers',
  summary: 'Which login providers are enabled.',
  invokes: 'identity.account-login',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'GET:/v1/identity/login/local/authorize', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Local login page',
  summary: 'Fake login provider page (dev/test only); answers only if the local provider is enabled.',
  invokes: 'identity.account-login',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'GET:/v1/identity/me', transport: 'http', access: 'guest', status: 'implemented',
  name: 'My identity',
  summary: 'Returns the caller\'s user from the already-verified token; no domain call.',
  invokes: 'identity.authorize',
  networkLimit: DEFAULT_LIMIT
});

PH.endpoint({
  id: 'PUT:/v1/identity/me/display-name', transport: 'http', access: 'guest', status: 'implemented',
  name: 'Set display name',
  summary: 'Sets the caller\'s display name.',
  invokes: 'identity.set-display-name',
  networkLimit: DEFAULT_LIMIT
});

// ---------- games / authoring (authenticated, /v1) ----------
PH.endpoint({
  id: 'POST:/v1/games', transport: 'http', access: 'account', status: 'implemented',
  name: 'Create game',
  summary: 'Creates a game (optionally from a template) with its first draft version.',
  invokes: 'create-game', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games', transport: 'http', access: 'account', status: 'implemented',
  name: 'List my games',
  summary: 'Lists the caller\'s games.',
  invokes: 'game.game-card', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games/{g}', transport: 'http', access: 'account', status: 'implemented',
  name: 'Read game card',
  summary: 'Reads a game\'s card.',
  invokes: 'game.game-card', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'PATCH:/v1/games/{g}', transport: 'http', access: 'account', status: 'implemented',
  name: 'Update game card',
  summary: 'Edits a game\'s name, description or visibility.',
  invokes: 'game.game-card', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'PUT:/v1/games/{g}/logo', transport: 'http', access: 'account', status: 'implemented',
  name: 'Upload logo',
  summary: 'Uploads the card logo; bytes go through the backend.',
  invokes: 'game.game-card', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/logo/remove', transport: 'http', access: 'account', status: 'implemented',
  name: 'Remove logo',
  summary: 'Removes the card logo.',
  invokes: 'game.game-card', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/photos', transport: 'http', access: 'account', status: 'implemented',
  name: 'Upload photo',
  summary: 'Uploads a card photo (up to 8).',
  invokes: 'game.game-card', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/photos/{id}/remove', transport: 'http', access: 'account', status: 'implemented',
  name: 'Remove photo',
  summary: 'Removes a card photo.',
  invokes: 'game.game-card', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/publish', transport: 'http', access: 'account', status: 'implemented',
  name: 'Publish game',
  summary: 'Publishes the owner\'s open edition.',
  invokes: 'publish-game', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/editions', transport: 'http', access: 'account', status: 'implemented',
  name: 'Open edition',
  summary: 'Opens a new editable draft of an already-published game.',
  invokes: 'open-edition', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games/{g}/draft', transport: 'http', access: 'account', status: 'implemented',
  name: 'Read draft',
  summary: 'Reads the open draft\'s content; repairs it in Play first if missing.',
  invokes: 'read-draft-workspace', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/draft/changes', transport: 'http', access: 'account', status: 'implemented',
  name: 'Apply draft changes',
  summary: 'Applies file edits to the draft, as a new revision.',
  invokes: 'edit-draft', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/draft/restore', transport: 'http', access: 'account', status: 'implemented',
  name: 'Restore draft revision',
  summary: 'Restores a previous revision\'s content as a new revision.',
  invokes: 'edit-draft', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/draft/template', transport: 'http', access: 'account', status: 'implemented',
  name: 'Use template',
  summary: 'Resets the draft onto a template, as a new revision.',
  invokes: 'edit-draft', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games/{g}/draft/revisions', transport: 'http', access: 'account', status: 'implemented',
  name: 'List draft revisions',
  summary: 'Revisions with provenance, paginated.',
  invokes: 'read-draft-workspace', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games/{g}/draft/changed', transport: 'http', access: 'account', status: 'implemented',
  name: 'Read draft changes',
  summary: 'Changes since a given revision.',
  invokes: 'read-draft-workspace', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games/{g}/draft/diff', transport: 'http', access: 'account', status: 'implemented',
  name: 'Diff draft revisions',
  summary: 'Unified diff between two revisions.',
  invokes: 'read-draft-workspace', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games/{g}/draft/files', transport: 'http', access: 'account', status: 'implemented',
  name: 'Read draft files',
  summary: 'File contents of a revision.',
  invokes: 'read-draft-workspace', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games/{g}/draft/build', transport: 'http', access: 'account', status: 'implemented',
  name: 'Read draft build',
  summary: 'Build state for the current revision; may long-poll up to 20 s.',
  invokes: 'read-draft-workspace', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/draft/simulations', transport: 'http', access: 'account', status: 'implemented',
  name: 'Simulate draft',
  summary: 'Runs a scenario (or up to 10 saved ones) against the draft.',
  invokes: 'simulate-draft', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games/{g}/draft/simulations/{id}/state', transport: 'http', access: 'account', status: 'implemented',
  name: 'Read simulation state',
  summary: 'Global or per-player state of a kept simulation result (30 min).',
  invokes: 'simulate-draft', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/draft/previews', transport: 'http', access: 'account', status: 'implemented',
  name: 'Start draft preview',
  summary: 'Opens a live preview of the draft\'s current build.',
  invokes: 'start-draft-preview', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'PUT:/v1/games/{g}/draft/assets/{key}', transport: 'http', access: 'account', status: 'implemented',
  name: 'Upload draft asset',
  summary: 'Uploads an asset of the draft, as a new revision.',
  invokes: 'edit-draft', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/draft/assets/{key}/remove', transport: 'http', access: 'account', status: 'implemented',
  name: 'Remove draft asset',
  summary: 'Removes an asset of the draft, as a new revision.',
  invokes: 'edit-draft', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/templates', transport: 'http', access: 'account', status: 'implemented',
  name: 'Featured templates',
  summary: 'Featured templates and categories (Discovery catalog is empty today).',
  invokes: 'browse-game-templates', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/templates/search', transport: 'http', access: 'account', status: 'implemented',
  name: 'Search templates',
  summary: 'Search templates by text, category, players, page (Discovery catalog is empty today).',
  invokes: 'browse-game-templates', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/templates/{id}', transport: 'http', access: 'account', status: 'implemented',
  name: 'Inspect template',
  summary: 'Card, brief, manifest and declared config of a template; never its code.',
  invokes: 'browse-game-templates', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/templates/{id}/files', transport: 'http', access: 'account', status: 'implemented',
  name: 'Read template files',
  summary: 'Reads 1 to 50 files of a template.',
  invokes: 'browse-game-templates', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/ai-link', transport: 'http', access: 'account', status: 'implemented',
  name: 'Create AI link',
  summary: 'Creates (or renews) the 7-day external-AI link to this game.',
  invokes: 'game-ai-link', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/games/{g}/ai-link', transport: 'http', access: 'account', status: 'implemented',
  name: 'AI link status',
  summary: 'Whether a live external-AI link exists for this game.',
  invokes: 'ai-link-status', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/games/{g}/ai-link/revoke', transport: 'http', access: 'account', status: 'implemented',
  name: 'Revoke AI link',
  summary: 'Revokes the external-AI link to this game.',
  invokes: 'game-ai-link', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'PUT:/v1/asset-uploads/{token}', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'Claim asset upload grant',
  summary: 'Single-use upload URL claimed by a resource grant.',
  invokes: 'edit-draft', networkLimit: '10/s and 120/min per client.'
});

// ---------- games / authoring (external-AI link, /ai/v1/{token}) ----------
PH.endpoint({
  id: 'GET:/ai/v1/{token}', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI context page',
  summary: 'Markdown instructions and operation URLs for the linked draft, or playhoot.ai-context.v1 JSON with ?format=json.',
  invokes: 'read-draft-workspace', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/files/{path...}', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI read file',
  summary: 'Reads one draft file.', invokes: 'read-draft-workspace', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/files', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI read files',
  summary: 'Reads several draft files.', invokes: 'read-draft-workspace', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/changed', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI read changes', summary: 'Changes since a revision.', invokes: 'read-draft-workspace', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/diff', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI diff revisions', summary: 'Unified diff between two revisions.', invokes: 'read-draft-workspace', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/revisions', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI list revisions', summary: 'Revisions with provenance, paginated.', invokes: 'read-draft-workspace', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/build', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI read build', summary: 'Build state; may long-poll up to 20 s.', invokes: 'read-draft-workspace', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/sdk/{page...}', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI read SDK page', summary: 'One page of the SDK guide, Markdown.', invokes: 'play.session.read-sdk-guide', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'POST:/ai/v1/{token}/changes', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI apply changes', summary: 'Applies file edits to the draft, as a new revision.', invokes: 'external-ai-edit', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'POST:/ai/v1/{token}/restore', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI restore revision', summary: 'Restores a previous revision as a new one.', invokes: 'external-ai-edit', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'POST:/ai/v1/{token}/template', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI use template', summary: 'Resets the draft onto a template, as a new revision.', invokes: 'external-ai-edit', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/templates', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI featured templates', summary: 'Featured templates and categories.', invokes: 'browse-game-templates', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/templates/search', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI search templates', summary: 'Search templates.', invokes: 'browse-game-templates', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/templates/{id}', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI inspect template', summary: 'Card, brief, manifest and declared config.', invokes: 'browse-game-templates', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/templates/{id}/files', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI read template files', summary: 'Reads 1 to 50 files of a template.', invokes: 'browse-game-templates', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'POST:/ai/v1/{token}/simulations', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI simulate', summary: 'Runs a scenario (or saved ones) against the draft.', invokes: 'simulate-draft', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'GET:/ai/v1/{token}/simulations/{id}/state', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI read simulation state', summary: 'Global or per-player state of a kept result.', invokes: 'simulate-draft', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'PUT:/ai/v1/{token}/assets/{key}', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI upload asset', summary: 'Uploads an asset of the draft, as a new revision.', invokes: 'external-ai-edit', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'POST:/ai/v1/{token}/assets/{key}/remove', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI remove asset', summary: 'Removes an asset of the draft, as a new revision.', invokes: 'external-ai-edit', networkLimit: AI_LINK_LIMIT
});
PH.endpoint({
  id: 'POST:/ai/v1/{token}/assets/{key}/upload-url', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'AI request asset upload URL',
  summary: 'Single-use grant URL so the AI uploads an asset without putting bytes through its own context.',
  invokes: 'external-ai-edit', networkLimit: AI_LINK_LIMIT
});

// ---------- OAuth for AI clients ----------
PH.endpoint({
  id: 'GET:/.well-known/oauth-protected-resource', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Protected resource metadata', summary: 'RFC 9728 metadata for the API as an OAuth resource.',
  invokes: 'identity.ai-oauth-reads', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/.well-known/oauth-protected-resource/mcp', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Protected resource metadata (MCP)', summary: 'RFC 9728 metadata for the MCP endpoint.',
  invokes: 'identity.ai-oauth-reads', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/.well-known/oauth-authorization-server', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Authorization server metadata', summary: 'RFC 8414 metadata.',
  invokes: 'identity.ai-oauth-reads', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/oauth/register', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Register AI client', summary: 'Dynamic client registration (RFC 7591), public clients only.',
  invokes: 'identity.ai-oauth', networkLimit: '5/min and 30/h per client IP.'
});
PH.endpoint({
  id: 'GET:/oauth/authorize', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Authorize AI client', summary: 'Redirects to the web consent page for the user to approve or deny.',
  invokes: 'identity.ai-oauth', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/oauth/token', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Exchange AI token', summary: 'Authorization code + PKCE, or refresh token, for an access token.',
  invokes: 'identity.ai-oauth', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/oauth/revoke', transport: 'http', access: 'anonymous', status: 'implemented',
  name: 'Revoke AI token', summary: 'RFC 7009 token revocation.',
  invokes: 'identity.ai-oauth', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/ai-authorizations/{requestID}', transport: 'http', access: 'account', status: 'implemented',
  name: 'Read AI authorization request', summary: 'The pending consent request\'s details.',
  invokes: 'identity.ai-oauth-reads', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/ai-authorizations/{requestID}/approve', transport: 'http', access: 'account', status: 'implemented',
  name: 'Approve AI authorization', summary: 'Approves the pending consent request.',
  invokes: 'identity.ai-oauth', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/ai-authorizations/{requestID}/deny', transport: 'http', access: 'account', status: 'implemented',
  name: 'Deny AI authorization', summary: 'Denies the pending consent request.',
  invokes: 'identity.ai-oauth', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'GET:/v1/ai-connections', transport: 'http', access: 'account', status: 'implemented',
  name: 'List AI connections', summary: 'The caller\'s approved AI connections.',
  invokes: 'identity.ai-oauth-reads', networkLimit: DEFAULT_LIMIT
});
PH.endpoint({
  id: 'POST:/v1/ai-connections/{id}/revoke', transport: 'http', access: 'account', status: 'implemented',
  name: 'Revoke AI connection', summary: 'Revokes an approved AI connection.',
  invokes: 'identity.ai-oauth', networkLimit: DEFAULT_LIMIT
});

// ---------- MCP transport ----------
PH.endpoint({
  id: 'POST:/mcp', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'MCP request', summary: 'Streamable HTTP MCP request; verifies the AI access token on every call.',
  invokes: 'identity.ai-oauth-reads', networkLimit: MCP_LIMIT
});
PH.endpoint({
  id: 'GET:/mcp', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'MCP stream', summary: 'Streamable HTTP MCP server-to-client stream.',
  invokes: 'identity.ai-oauth-reads', networkLimit: MCP_LIMIT
});
PH.endpoint({
  id: 'DELETE:/mcp', transport: 'http', access: 'external-ai', status: 'implemented',
  name: 'MCP session close', summary: 'Closes the stateless MCP exchange.',
  invokes: 'identity.ai-oauth-reads', networkLimit: MCP_LIMIT
});

// ---------- client telemetry (owned by another agent; listed for completeness) ----------
PH.endpoint({
  id: 'POST:/v1/client-telemetry', transport: 'http', access: 'guest', status: 'implemented',
  name: 'Report client telemetry',
  summary: 'Batch of client telemetry events (<=100/16 KiB); always 204. Event catalog and storage are outside identity/game/play, not detailed here.',
  invokes: 'identity.authorize', networkLimit: DEFAULT_LIMIT
});

// ---------- MCP tools (22) ----------
PH.endpoint({ id: 'MCP:list_my_games', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'list_my_games', summary: 'Lists the user\'s games.', invokes: 'game.game-card', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:create_game', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'create_game', summary: 'Creates an unpublished game (optional template).', invokes: 'create-game', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:browse_templates', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'browse_templates', summary: 'Featured templates and categories.', invokes: 'browse-game-templates', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:search_templates', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'search_templates', summary: 'Search by text, category, players, page.', invokes: 'browse-game-templates', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:inspect_template', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'inspect_template', summary: 'Card, brief, manifest, files; never the code.', invokes: 'browse-game-templates', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:read_template_files', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'read_template_files', summary: 'Reads 1 to 50 files of a template.', invokes: 'browse-game-templates', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:use_template', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'use_template', summary: 'Resets the draft onto a template, new revision.', invokes: 'edit-draft', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:get_game_context', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'get_game_context', summary: 'Revision, files, build, assets, attempts, notes.md, brief.json, SDK guide URL.', invokes: 'read-draft-workspace', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:read_files', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'read_files', summary: 'File contents of a revision.', invokes: 'read-draft-workspace', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:list_changes_since', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'list_changes_since', summary: 'Changes since a revision.', invokes: 'read-draft-workspace', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:apply_changes', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'apply_changes', summary: 'Find/replace edits with base_revision and idempotency_key.', invokes: 'edit-draft', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:restore_revision', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'restore_revision', summary: 'New revision with a previous one\'s content.', invokes: 'edit-draft', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:list_revisions', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'list_revisions', summary: 'Revisions with provenance, paginated.', invokes: 'read-draft-workspace', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:diff_revisions', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'diff_revisions', summary: 'Unified diff between two revisions (8 KiB/file, 64 KiB total).', invokes: 'read-draft-workspace', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:get_sdk_index', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'get_sdk_index', summary: 'Compact index of the SDK guide.', invokes: 'play.session.read-sdk-guide', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:get_sdk_doc', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'get_sdk_doc', summary: 'One page of the SDK guide, Markdown.', invokes: 'play.session.read-sdk-guide', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:request_asset_upload', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'request_asset_upload', summary: 'Single-use upload URL so the AI skips putting bytes through its context.', invokes: 'edit-draft', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:upload_asset', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'upload_asset', summary: 'Uploads a small file (<=256 KiB) in base64.', invokes: 'edit-draft', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:remove_asset', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'remove_asset', summary: 'Removes the asset at a key, new revision.', invokes: 'edit-draft', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:get_build', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'get_build', summary: 'Build state, with wait_seconds up to 20.', invokes: 'read-draft-workspace', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:simulate', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'simulate', summary: 'Runs a scenario (virtual clock, expectations, holds) or saved ones.', invokes: 'simulate-draft', networkLimit: MCP_LIMIT });
PH.endpoint({ id: 'MCP:read_simulation_state', transport: 'mcp', access: 'external-ai', status: 'implemented', name: 'read_simulation_state', summary: 'Global or per-player state of a kept result (30 min).', invokes: 'simulate-draft', networkLimit: MCP_LIMIT });
