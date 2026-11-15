// Domain: composer. Cross-domain reads; no tables of its own. Detail is loaded by a later task.
PH.domain({
  id: 'composer', parent: null, kind: 'composer', status: 'implemented',
  name: 'Composer',
  summary: 'Answers reads that need several top-level domains; stateless, read-only.',
  responsibilities: [
    'Ask the owning domain for permission first, then read the others.',
    'Join entities with the notifications and references that point at them.',
    'Build the frontend destination URL of each notification or composed item (the URL builder lives here, not in domains).',
    'Expose read endpoints shaped for screens (backend-for-consumer).'
  ],
  doesNotKnow: ['Tables of any domain.', 'Write operations of any kind.'],
  sources: ['B: composer', 'P14§4-6']
});
