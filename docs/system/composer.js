// Domain: composer. Cross-domain reads; no tables of its own. Detail is loaded by a later task.
PH.domain({
  id: 'composer', parent: null, kind: 'composer', status: 'implemented',
  name: 'Composer',
  summary: 'Answers reads that need several top-level domains; stateless.',
  responsibilities: ['Ask the owning domain for permission first, then read the others.'],
  doesNotKnow: ['Tables of any domain.'],
  sources: []
});
