// Shared technical packages. They are not domains: any domain may use them directly,
// they hold no business rules, and their tables (if any) are infrastructure.
PH.package({
  id: 'txqueue', status: 'planned',
  name: 'Transactional queue',
  summary: 'The one way to do async work: a queue table and worker per domain, with a dead letter queue.',
  provides: [
    'Enqueue a task in the same transaction as the write that caused it.',
    'One infrastructure table per domain (<domain>_queue_tasks): task uuid, type, payload, run_at, attempts, lock, status pending | running | dead | cancelled; completed tasks are deleted.',
    'A worker per domain: picks due tasks with SKIP LOCKED, runs the registered handler, retries with exponential backoff and jitter; after max attempts the task turns dead (that is the dead letter queue).',
    'At-least-once delivery: every handler must be idempotent.',
    'Handlers are registered per task type by the owning domain; cross-domain delivery types (notify, analytics-event, spend-event) are registered by the orchestrator, so domains never call each other (D-010).',
    'Delayed tasks via run_at; periodic tasks re-enqueue themselves on completion.',
    'A registry over every domain queue for admin: list dead tasks, retry (back to pending), cancel. Reached through flows whose step names this package (D-013).',
    'Service, queue and processor code kept separate, ready to migrate to a real outbox and queue.'
  ],
  doesNotKnow: ['What a task means: each domain owns its task types and handlers.'],
  sources: ['B: principles', 'P32§6']
});

PH.package({
  id: 'limits', status: 'implemented',
  name: 'Windowed limits',
  summary: 'Counting things per time window, for network limits and logical business limits.',
  provides: [
    'Multi-window counters (per second, minute, hour, rolling days).',
    'The same mechanics for API network limits and each domain\'s own logical limits.'
  ],
  doesNotKnow: ['Plan capacities: those arrive as values from the caller and each domain compares usage itself.'],
  sources: ['B: business rules', 'B: plan']
});

PH.package({
  id: 'storage', status: 'implemented',
  name: 'Object storage',
  summary: 'Files behind a port: scripts, frontend packages, assets, replay chunks.',
  provides: [
    'Put, get and delete objects behind an interface (local emulator today, a real bucket later).',
    'Signed URLs only when the bytes never pass through the backend; otherwise the backend reads them.'
  ],
  doesNotKnow: ['What an object contains: each domain names and interprets its own objects.'],
  sources: ['B: principles']
});

PH.package({
  id: 'logging', status: 'implemented',
  name: 'Logging',
  summary: 'Structured request logs that let one request be followed across domains.',
  provides: [
    'One log per request with its steps, enough to reconstruct the path across domains.',
    'No credentials, private materials or unnecessary student data in logs.'
  ],
  doesNotKnow: ['Business meaning of a step.'],
  sources: ['B: principles', 'P16§5']
});

PH.package({
  id: 'metrics', status: 'implemented',
  name: 'Metrics and alerts',
  summary: 'Per-endpoint network metrics, one logical-outcome metric per domain operation, and actionable alerts.',
  provides: [
    'HTTP metrics at the network level (path, time, code) reported by api.',
    'A logical-outcome metric declared by each domain operation.',
    'Alerts for the situations the specification marks as actionable (writes to the log today; a real backend arrives with delivery 2).'
  ],
  doesNotKnow: ['What to do about an alert: admin and the runbooks decide.'],
  sources: ['B: principles', 'P16§5']
});
