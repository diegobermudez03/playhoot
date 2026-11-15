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
