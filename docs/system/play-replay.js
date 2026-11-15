// Subdomain: play.replay (planned; recording exists inside runtime today). Detailed by the H5 design.
PH.domain({
  id: 'play.replay', parent: 'play', kind: 'traditional', status: 'planned',
  name: 'Play replay',
  summary: 'Recordings of Sessions: storage, playback, retention and identity substitution.',
  responsibilities: [
    'Store the best-effort recording each Session produces and say when segments are missing.',
    'Play a Session back like a video (play, seek), per perspective.',
    'Apply the perspective permissions the Session had: a player sees their own, a directing host sees all, a playing host only their own.',
    'Retain replays 12 months from the Session, then purge them.',
    'Replace a deleted or expired identity with a neutral, distinguishable reference, also inside the replayed content.',
    'Hide affected content on a security or legal order; restrict the whole replay with a reason when hiding is not enough.'
  ],
  doesNotKnow: [
    'Grades: they are never reconstructed from a replay.',
    'Why an identity was removed (identity/compliance decide; replay substitutes).'
  ],
  references: [
    'Play session: the Session each recording belongs to and its perspective permissions.',
    'Identity: participants, until substitution.'
  ],
  sources: ['B: history', 'P02§10', 'P06§1', 'P06§11', 'P13§5']
});
