// Shared technical packages. They are not domains: any domain may use them directly,
// they hold no business rules, and their tables (if any) are infrastructure.
PH.package({
  id: 'txqueue', status: 'implemented',
  name: 'Transactional queue',
  summary: 'The one way to do async work: a queue table and worker per domain, with a dead letter queue.',
  provides: [
    'Enqueue a task in the same transaction as the write that caused it.',
    'One infrastructure table per domain (<domain>_queue_tasks), created by a migration helper in the own migrations of each domain: task uuid, type, payload, optional idempotency key (one live task per type and key), run_at, attempts, last error, admin retries, status pending | running | dead | cancelled; completed tasks are deleted, cancelled ones kept.',
    'A worker per domain on every instance: claims due tasks with SKIP LOCKED under a 5-minute lease (the task of a crashed worker is claimed again when the lease ends), runs the registered handler, retries with exponential backoff and jitter (V-02: 10 attempts, 10 s doubling to 1 h); after max attempts, or a failure the handler marks permanent, the task turns dead (that is the dead letter queue).',
    'At-least-once delivery: every handler must be idempotent.',
    'Handlers are registered per task type by the owning domain; cross-domain delivery types (notify, analytics-event, spend-event) are registered by the orchestrator, so domains never call each other (D-010).',
    'Delayed tasks via run_at; periodic tasks are scheduled once when the worker starts and re-enqueue themselves on completion. Tasks of a type nobody handles yet wait in the queue.',
    'A registry over every domain queue for admin: list dead tasks (newest 100 per queue), retry (back to pending with fresh attempts), cancel. Reached through flows whose step names this package (D-013).',
    'Service, queue and processor code kept separate, ready to migrate to a real outbox and queue: domain services only enqueue inside their transaction; the queue owns the table; the processor (worker) claims, runs handlers and records outcomes; handlers are plain functions registered per type.'
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
