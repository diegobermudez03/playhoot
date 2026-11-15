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

PH.endpoint({
  id: 'POST:/v1/sessions', transport: 'http', access: 'account', status: 'implemented',
  name: 'Create session',
  summary: 'Creates a Session in lobby for a game the caller may play.',
  invokes: 'create-session',
  networkLimit: 'Default: 5/s and 30/min per user; 20/s and 300/min per client address.'
});
