// Domain: moderation (planned). Publication review core in the H4 design; measures and appeals in H9.
PH.domain({
  id: 'moderation', parent: null, kind: 'traditional', status: 'planned',
  name: 'Moderation',
  summary: 'Decisions about content and conduct: the publication review queue, graduated measures and appeal outcomes.',
  responsibilities: [
    'Run the publication review queue: approve or deny a specific combination of version, public card and assets; approving one approves nothing later.',
    'Keep Discovery on the previously approved combination while an update is under review; never approve automatically, even saturated.',
    'Run the initial automatic classification of content and reports (moderation model behind a port); support conversations and student data are never sent to it.',
    'Decide and record graduated measures: remove from Discovery, prevent new sessions, stop a session, restrict a version, suspend an account; execution happens in the owning domain through orchestration.',
    'Record every decision with reason, scope, duration and appeal path; a final account sanction requires human review; decisions are not disclosed to reporters.',
    'Apply preventive restrictions from flags and mass reports without sanctioning on reports alone.',
    'Record automatic system actions and their admin acknowledgement.',
    'Decide appeals (the case conversation lives in support).'
  ],
  doesNotKnow: [
    'Catalog data, cards or version content (game, play.versions): it references what it judges.',
    'Case conversations and tracking (support).',
    'How a suspension affects renewals (billing reacts through orchestration).'
  ],
  references: [
    'Game, Play: the reviewed or restricted entity (external identifiers).',
    'Identity: the sanctioned or reviewing person (user identifier).',
    'Support: the report or appeal that triggered a decision (case identifier).'
  ],
  sources: ['P04§4', 'P13§1-6', 'P24§1', 'B: business rules']
});

// ---------- H4 (T-400 design): the publication review core. H9 completes the domain
// (automatic classification, the other graduated measures, appeals, system actions).

var REVIEW_LIFECYCLE = {
  states: ['PENDING', 'APPROVED', 'DENIED', 'CANCELLED'],
  transitions: [
    { from: 'PENDING', to: 'APPROVED', on: 'the admin approves the exact combination' },
    { from: 'PENDING', to: 'DENIED', on: 'the admin denies it with a note' },
    { from: 'PENDING', to: 'CANCELLED', on: 'the request was superseded, withdrawn, or its game deleted or blocked' }
  ]
};

PH.table({
  id: 'moderation.moderation_review_tasks', domain: 'moderation', status: 'planned',
  name: 'moderation_review_tasks',
  summary: 'The publication review queue: one task per submitted version + card + assets combination.',
  purpose: 'Never approved automatically, even saturated (P04§4, SPEC_MAP §18); the decision references the combination, game holds the snapshot.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'uuid', type: 'uuid', note: '' },
    { name: 'kind', type: 'text', note: 'publication (H9 adds more).' },
    { name: 'game_uuid', type: 'uuid', note: 'The judged game.' },
    { name: 'publication_uuid', type: 'uuid', note: 'The exact combination submitted.' },
    { name: 'status', type: 'text', note: 'See lifecycle.' },
    { name: 'note', type: 'text', note: 'The note to the creator on a denial.' },
    { name: 'decided_by_user_uuid', type: 'uuid', note: 'The admin; empty until decided.' },
    { name: 'decided_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (uuid)', 'UNIQUE (publication_uuid)', 'partial (created_at) while PENDING (the queue)', '(game_uuid)'],
  history: 'Append-only decision columns (write-once) plus moderation_review_status_changes.',
  references: [
    { column: 'game_uuid', to: 'game game (external identifier)' },
    { column: 'publication_uuid', to: 'game publication (external identifier)' },
    { column: 'decided_by_user_uuid', to: 'identity user' }
  ],
  lifecycle: REVIEW_LIFECYCLE
});

PH.table({
  id: 'moderation.moderation_review_status_changes', domain: 'moderation', status: 'planned',
  name: 'moderation_review_status_changes',
  summary: 'Every status a review task went through.',
  purpose: 'Append-only history.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'review_task_id', type: 'bigint', note: '' },
    { name: 'from_status', type: 'text', note: 'Empty for the creation.' },
    { name: 'to_status', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(review_task_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'review_task_id', to: 'moderation.moderation_review_tasks' }]
});

PH.table({
  id: 'moderation.moderation_measures', domain: 'moderation', status: 'planned',
  name: 'moderation_measures',
  summary: 'Every graduated measure decided, with reason and scope; H4 uses only block-game and unblock-game.',
  purpose: 'The decision record P13§6 asks for: reason, scope, duration when it applies, and later the appeal path (H9). Execution happens in the owning domain through orchestration.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'uuid', type: 'uuid', note: 'What the executing domain references.' },
    { name: 'kind', type: 'text', note: 'block-game or unblock-game (H9 adds the rest of P13§5).' },
    { name: 'subject_kind', type: 'text', note: 'game (H9 adds version, session, account).' },
    { name: 'subject_uuid', type: 'uuid', note: '' },
    { name: 'reason_code', type: 'text', note: 'The kind shown to the affected person.' },
    { name: 'note', type: 'text', note: 'Internal detail; never shown.' },
    { name: 'decided_by_user_uuid', type: 'uuid', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (uuid)', '(subject_kind, subject_uuid)'],
  history: 'It is itself the history (append-only: an unblock is a new measure).',
  references: [
    { column: 'subject_uuid', to: 'game game (external identifier)' },
    { column: 'decided_by_user_uuid', to: 'identity user' }
  ]
});

PH.operation({
  id: 'moderation.submit-review', domain: 'moderation', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'SubmitReview',
  summary: 'Opens a PENDING review task for a submitted publication; supersession cancels the previous task of the game.',
  reads: ['moderation.moderation_review_tasks'],
  writes: ['moderation.moderation_review_tasks', 'moderation.moderation_review_status_changes'],
  rules: [
    'Private games never generate a task: only a publication request reaches here (P13§3).',
    'One PENDING task per game: submitting an update cancels the previous task (the publication row was superseded in game).',
    'Idempotent by publication uuid.'
  ],
  lifecycle: REVIEW_LIFECYCLE,
  sources: ['P04§4', 'P13§3']
});

PH.operation({
  id: 'moderation.decide-review', domain: 'moderation', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'DecideReview',
  summary: 'The admin approves or denies the exact submitted combination; deny carries a note to the creator.',
  reads: ['moderation.moderation_review_tasks'],
  writes: ['moderation.moderation_review_tasks', 'moderation.moderation_review_status_changes'],
  rules: [
    'Approving approves this combination only — not the next version nor a card edited afterwards (P04§4).',
    'Never automatic: a saturated queue pauses or communicates the wait, it does not approve (SPEC_MAP §18).',
    'The decision records actor and time; the reporter of anything is never told (P13§6).',
    'Deciding an already-decided task is idempotent; a CANCELLED task refuses.'
  ],
  sources: ['P04§4', 'P13§6']
});

PH.operation({
  id: 'moderation.cancel-review', domain: 'moderation', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'CancelReview',
  summary: 'Cancels a PENDING task whose request was superseded or withdrawn, or whose game was deleted or blocked.',
  reads: ['moderation.moderation_review_tasks'],
  writes: ['moderation.moderation_review_tasks', 'moderation.moderation_review_status_changes'],
  rules: ['Idempotent; a decided task is left as it is.'],
  sources: ['P04§4']
});

PH.operation({
  id: 'moderation.list-reviews', domain: 'moderation', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'ListReviews',
  summary: 'The pending queue (oldest first) and the decided history, for the admin panel.',
  reads: ['moderation.moderation_review_tasks'],
  writes: [],
  rules: ['The composed admin view joins each task\'s snapshot, screenshots and version facts from game and play.versions.'],
  limits: [{ what: 'Page size', value: '50 (V-13)', source: 'domain' }],
  sources: ['P04§4', 'B: admin']
});

PH.operation({
  id: 'moderation.record-measure', domain: 'moderation', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'RecordMeasure',
  summary: 'Records a block-game or unblock-game measure with reason and actor; the flow then executes it in game.',
  reads: ['moderation.moderation_measures'],
  writes: ['moderation.moderation_measures'],
  rules: [
    'The administrative block is distinct from the creator\'s withdrawal and from deletion (P04§6); the sufficient measure is applied, nothing more (P13§5).',
    'Blocking also cancels the game\'s pending review task.',
    'Isolated or mass reports never block on their own (P13§4) — reports arrive with H9; in H4 the admin acts directly.',
    'Appeals and the rest of the graduated ladder are H9.'
  ],
  sources: ['P04§6', 'P13§4-6']
});
