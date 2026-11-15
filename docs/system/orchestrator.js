// Domain: orchestrator. Cross-domain writes; no tables of its own.
PH.domain({
  id: 'orchestrator', parent: null, kind: 'orchestrator', status: 'implemented',
  name: 'Orchestrator',
  summary: 'Coordinates writes that cross top-level domains; may persist the steps of flows that need recovery.',
  responsibilities: [
    'Call domains in order for operations that cross them; only it and composer may combine domains.',
    'Pass idempotency keys through; retries are completed by the domains.',
    'Fetch plan capacities and budget admission and pass them to the owning domain, which applies the stricter limit.',
    'Record the steps of flows that must recover or compensate (payments, grades), and finish incomplete ones without duplicating effects.'
  ],
  doesNotKnow: ['Tables of any domain.', 'Rules that belong to a single domain.'],
  sources: ['B: orchestrator', 'P15§6']
});

PH.flow({
  id: 'create-session', status: 'implemented',
  name: 'Create session',
  summary: 'Checks that the caller may play the game, then creates a Session in lobby with a join code.',
  coordinator: 'orchestrator', effect: 'write',
  steps: [
    { n: 1, domain: 'game', operation: 'game.check-playability', when: 'always', onFailure: 'Stop with game_unavailable.' },
    { n: 2, domain: 'play.session', operation: 'play.session.create-session', when: 'the game is playable', onFailure: 'Return its outcome: active_session_limit, game_unavailable or idempotency conflict.' }
  ],
  persistence: 'none',
  idempotency: 'The Idempotency-Key is passed to Play, which records each request once per user and key.',
  compensation: 'None needed: step 1 only reads.',
  sources: ['P04§3', 'P06']
});

PH.flow({
  id: 'edit-draft', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Edit draft (web or API)',
  summary: 'The owner applies changes, restores a revision, uploads or removes an asset, or starts from a template, on their own open draft.',
  steps: [
    { n: 1, domain: 'game', operation: 'game.find-editable-version', when: 'always', onFailure: 'Stop with not_found or not_editable.' },
    { n: 2, domain: 'play.versions', operation: 'play.versions.apply-changes', when: 'Play already has a draft for the game\'s open edition', onFailure: 'If Play has no draft yet (ErrDraftNotFound), create one and repeat once.' }
  ],
  persistence: 'none',
  idempotency: 'The caller\'s Idempotency-Key is recorded once per draft version and key.',
  compensation: 'None needed: each step is transactional within its own domain.',
  sources: ['P04§1', 'P09§2-3']
});

PH.flow({
  id: 'ensure-draft', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Ensure draft exists',
  summary: 'Authorizes the owner and creates the draft in Play if Game already has an open edition but Play does not, so a composer read can proceed.',
  steps: [
    { n: 1, domain: 'game', operation: 'game.find-editable-version', when: 'always', onFailure: 'Stop with not_found or not_editable.' },
    { n: 2, domain: 'play.versions', operation: 'play.versions.create-draft-version', when: 'Play has no draft for that edition yet', onFailure: 'None: creation is itself the repair.' }
  ],
  persistence: 'none',
  idempotency: 'Natural: creating a draft that already exists is a no-op.',
  compensation: 'None needed.',
  sources: ['B: play']
});

PH.flow({
  id: 'start-draft-preview', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Start draft preview',
  summary: 'Opens a live preview of the draft\'s current build, after repairing authorization and the draft if needed.',
  steps: [
    { n: 1, domain: 'game', operation: 'game.find-editable-version', when: 'always', onFailure: 'Stop with not_found or not_editable.' },
    { n: 2, domain: 'play.versions', operation: 'play.versions.create-draft-version', when: 'Play has no draft yet', onFailure: 'None: creation is itself the repair.' },
    { n: 3, domain: 'play.versions', operation: 'play.versions.open-preview', when: 'always', onFailure: 'Stop with unavailable if there is no successful build yet.' }
  ],
  persistence: 'none',
  idempotency: 'Opening a preview while one is live for the version replaces it (end_reason replaced).',
  compensation: 'None needed.',
  sources: ['P09§4']
});

PH.flow({
  id: 'game-ai-link', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Create or revoke external-AI link',
  summary: 'The owner creates a 7-day read/write link to their draft for an external AI, or revokes it.',
  steps: [
    { n: 1, domain: 'game', operation: 'game.find-editable-version', when: 'always', onFailure: 'Stop if the caller does not own the game.' },
    { n: 2, domain: 'identity', operation: 'identity.resource-link', when: 'always', onFailure: 'Propagate Identity\'s error.' }
  ],
  persistence: 'none',
  idempotency: 'Creating another link while one is live revokes the previous one (renewal).',
  compensation: 'None needed.',
  sources: ['P10§1-3']
});

PH.flow({
  id: 'publish-game', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Publish game',
  summary: 'Publishes the owner\'s open edition: freezes the draft and marks the game published. There is no review step today (see GAPS: request-publication / review-publication are not implemented).',
  steps: [
    { n: 1, domain: 'game', operation: 'game.find-editable-version', when: 'always', onFailure: 'Stop with not_found or not_editable.' },
    { n: 2, domain: 'play.versions', operation: 'play.versions.read-draft', when: 'always (CheckPublication)', onFailure: 'Stop with the publication blocker (missing build, missing assets, revision changed).' },
    { n: 3, domain: 'game', operation: 'game.game-lifecycle', when: 'the draft is publishable (BeginPublication)', onFailure: 'Stop with name_required or a pending-release conflict.' },
    { n: 4, domain: 'play.versions', operation: 'play.versions.freeze-version', when: 'always', onFailure: 'Abandon the release in Game (AbandonPublication) if Play rejects it.' },
    { n: 5, domain: 'game', operation: 'game.game-lifecycle', when: 'the version froze', onFailure: 'n/a (CompletePublication).' }
  ],
  persistence: 'none',
  idempotency: 'A pending release is resumed on retry instead of starting another; best-effort cleanup of unused objects runs after a successful publish.',
  compensation: 'A Play rejection after the release is opened in Game abandons that release (AbandonPublication) instead of leaving it pending.',
  sources: ['P04§4']
});

PH.flow({
  id: 'open-edition', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Open new edition',
  summary: 'Opens a new editable draft of an already-published game, from its current version.',
  steps: [
    { n: 1, domain: 'game', operation: 'game.game-lifecycle', when: 'always (OpenEdition)', onFailure: 'Stop if there is already an open edition or a pending release.' },
    { n: 2, domain: 'play.versions', operation: 'play.versions.create-draft-version', when: 'the edition opened', onFailure: 'Retry with the same key completes the missing step.' }
  ],
  persistence: 'none',
  idempotency: 'Idempotent by key at the Game step (game_edition_requests).',
  compensation: 'None needed.',
  sources: ['P04§5']
});
