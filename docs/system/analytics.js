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
