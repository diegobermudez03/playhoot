// Domain: analytics (planned). Ingestion in the H1 design; reports in H10.
PH.domain({
  id: 'analytics', parent: null, kind: 'traditional', status: 'planned',
  name: 'Analytics',
  summary: 'Long-term numbers in their own tables, fed by events the domains enqueue; decoupled from the business logic.',
  responsibilities: [
    'Ingest events from every domain through the transactional queue into its own analysis tables.',
    'Answer activation, retention, frequency, creator churn, usage, revenue, costs and limit-hit conversions, split by Personal/Teacher and free/trial/subscription.',
    'Keep analytics queries off the operational tables.'
  ],
  doesNotKnow: [
    'Individual students: no individual tracking, no school data in commercial analytics.',
    'External analytics providers: there are none.'
  ],
  references: ['Any domain: the entities behind events (external identifiers), kept pseudonymous where the rules require.'],
  sources: ['B: principles', 'P34', 'P22§4']
});

// ---------- H1 design (planned): ingestion. Reports and admin queries are the H10 design. ----------

PH.table({
  id: 'analytics.analytics_events', domain: 'analytics', status: 'planned',
  name: 'analytics_events',
  summary: 'The event store the P34 metrics are computed from, decoupled from the operational tables.',
  purpose: 'Only the events and dates the activation, retention, frequency and churn metrics need; never every click.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'event_type', type: 'text', note: 'Constant: game-created, game-ready, game-edited, session-started, plus the commercial ones later milestones add.' },
    { name: 'user_ref', type: 'uuid', note: 'The creator the event belongs to; never a student.' },
    { name: 'account_kind', type: 'text', note: 'personal or teacher, at event time.' },
    { name: 'plan_kind', type: 'text', note: 'free, trial or subscription, at event time.' },
    { name: 'entity_ref', type: 'text', note: 'External identifier of the game or session.' },
    { name: 'properties', type: 'jsonb', note: 'Small, event-specific values (e.g. whether the session had real participants).' },
    { name: 'source_domain', type: 'text', note: '' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'occurred_at', type: 'timestamptz', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (source_domain, idempotency_key)', '(event_type, occurred_at)', '(user_ref, occurred_at)'],
  history: 'It is itself the history (append-only).',
  references: []
});

PH.operation({
  id: 'analytics.ingest-events', domain: 'analytics', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'IngestEvents',
  summary: 'Stores a batch of events from the queue, idempotently.',
  reads: ['analytics.analytics_events'],
  writes: ['analytics.analytics_events'],
  rules: [
    'Idempotent by (source domain, idempotency key) per event.',
    'No individual student tracking, answers, grades or materials; internal tests are marked so they are never shown as real use.',
    'Ingestion respects the economic limits of P32 like any other queue work.'
  ],
  sources: ['P34§2-4', 'P22§4']
});
