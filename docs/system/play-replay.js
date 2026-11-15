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

PH.table({
  id: 'play.replay.play_replays', domain: 'play.replay', status: 'implemented',
  name: 'play_replays',
  summary: 'The replay manifest for one Session.',
  purpose: 'Finalized once the Session ends and its last chunk is written.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'session_id', type: 'bigint', note: '' },
    { name: 'final_session_time_ms', type: 'bigint', note: '' },
    { name: 'finalized_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (session_id)'],
  history: 'Write-once: finalized_at.',
  references: [{ column: 'session_id', to: 'play.session.play_sessions' }]
});

PH.table({
  id: 'play.replay.play_replay_chunks', domain: 'play.replay', status: 'implemented',
  name: 'play_replay_chunks',
  summary: 'One recorded chunk of a replay (60 s of game time, zstd-compressed, in object storage).',
  purpose: 'Chunks are produced in order; the last one is marked final.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'recording_uuid', type: 'uuid', note: '' },
    { name: 'chunk_index', type: 'int', note: '' },
    { name: 'started_at_ms, ended_at_ms', type: 'bigint, bigint', note: 'Game-time range.' },
    { name: 'final', type: 'boolean', note: '' },
    { name: 'object_key', type: 'text', note: '' },
    { name: 'sha256', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (recording_uuid, chunk_index)'],
  history: 'infrastructure',
  references: [{ column: 'recording_uuid', to: 'play.replay.play_replays' }]
});

PH.operation({
  id: 'play.replay.store-replay-chunk', domain: 'play.replay', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'storereplaychunk (StartRecording / Offer / Finish / StopRecording / Close)',
  summary: 'Writes a replay chunk\'s bytes to object storage, then registers it.',
  reads: ['play.replay.play_replays'],
  writes: ['play.replay.play_replays', 'play.replay.play_replay_chunks'],
  rules: [
    'A bounded in-memory queue per instance holds chunks waiting to be stored; the oldest are dropped if it fills up (64 pending, 5 attempts).',
    'Producer side only: there is no playback and no retention sweep yet.'
  ],
  limits: [{ what: 'Chunk duration', value: '60 s', source: 'domain' }],
  sources: []
});
