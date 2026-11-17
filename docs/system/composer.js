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

PH.flow({
  id: 'ai-link-status', status: 'implemented', coordinator: 'composer', effect: 'read',
  name: 'External-AI link status',
  summary: 'Tells the owner whether a live external-AI link exists for their game.',
  steps: [
    { n: 1, domain: 'game', operation: 'game.find-editable-version', when: 'always', onFailure: 'Stop if the caller does not own the game.' },
    { n: 2, domain: 'identity', operation: 'identity.resource-link-status', when: 'always', onFailure: 'n/a' }
  ],
  persistence: 'none', idempotency: 'n/a (read-only)', compensation: 'none',
  sources: ['P10§1-3']
});

PH.flow({
  id: 'read-draft-workspace', status: 'implemented', coordinator: 'composer', effect: 'read',
  name: 'Read draft workspace',
  summary: 'Reads a draft\'s content, changes, diffs, revisions and build state for its owner or an authorized AI.',
  steps: [
    { n: 1, domain: 'game', operation: 'game.find-editable-version', when: 'always', onFailure: 'A version Game has but Play does not yet (Missing) is repaired by the ensure-draft flow and reread.' },
    { n: 2, domain: 'play.versions', operation: 'play.versions.read-draft', when: 'the version exists in Play', onFailure: 'n/a' }
  ],
  persistence: 'none', idempotency: 'n/a (read-only)', compensation: 'none',
  sources: ['P09§6', 'P10§4']
});

PH.flow({
  id: 'simulate-draft', status: 'implemented', coordinator: 'composer', effect: 'read',
  name: 'Simulate draft',
  summary: 'Runs up to 10 saved scenarios (or an ad hoc one) against the draft\'s current build and reads a kept result back.',
  steps: [
    { n: 1, domain: 'game', operation: 'game.find-editable-version', when: 'always', onFailure: 'Stop if the caller does not own the game.' },
    { n: 2, domain: 'play.runtime', operation: 'play.runtime.simulate', when: 'always', onFailure: 'n/a' }
  ],
  persistence: 'none', idempotency: 'n/a (execution does not persist outside the kept result)', compensation: 'none',
  sources: ['P09§6']
});

PH.flow({
  id: 'browse-game-templates', status: 'implemented', coordinator: 'composer', effect: 'read',
  name: 'Browse and inspect templates',
  summary: 'Featured and searched templates, and a template\'s card, manifest and files (never its code). The catalog behind it is a stub that returns empty lists today; H4 (T-404) backs it with the featured-template selection and the Discovery search\'s template filter (D-047/D-048).',
  steps: [
    { n: 1, domain: 'game', operation: 'game.find-template', when: 'inspecting or copying a specific template', onFailure: 'Stop with not_found.' }
  ],
  persistence: 'none', idempotency: 'n/a (read-only)', compensation: 'none',
  sources: ['B: search', 'P14§2']
});
