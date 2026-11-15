// Domain: support (planned; the base called it "report"). Detailed by the H9 design.
PH.domain({
  id: 'support', parent: null, kind: 'traditional', status: 'planned',
  name: 'Support',
  summary: 'Reports and help cases: typed reports, conversations, the public form and inbound email.',
  responsibilities: [
    'Keep typed, operation-aware reports (game, version, session, conduct, creation, host, general), each with its own data, relations and priority.',
    'Keep help cases (payments, technical, privacy, content, appeals) with reference, conversation and state; internal notes separate.',
    'Run the user lifecycle (open, awaiting reply, requiring input, closed) and the admin lifecycle (new, pending information, under review, resolved, appealed); acknowledgement is not resolution.',
    'Offer the public no-account form for illegal content, privacy, copyright and access problems, with secure tracking.',
    'Receive inbound support email (SES behind a port) into cases.',
    'Keep the frequently asked questions content.',
    'Track service targets per case type and delete ordinary cases 6 months after closing.',
    'Keep support and appeals available to suspended users.'
  ],
  doesNotKnow: [
    'Moderation decisions and measures (moderation): a case references them.',
    'The internals of reported entities: it stores their external identifiers.',
    'Automatic content classification (moderation runs it).'
  ],
  references: [
    'Identity: reporters and case owners (user identifier), when there is one.',
    'Game, Play, Classroom: reported entities (external identifiers).',
    'Moderation: the measure a case or appeal is about (measure identifier).'
  ],
  sources: ['B: report', 'P13§4-6', 'P33']
});
