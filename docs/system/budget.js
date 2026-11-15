// Domain: budget (planned). Detailed by the H1 design (minimal core) and the H6 design (complete).
PH.domain({
  id: 'budget', parent: null, kind: 'traditional', status: 'planned',
  name: 'Budget',
  summary: 'What Playhoot spends and can afford: measured costs, reserves, admission of cost-bearing work, and the emergency shutdown.',
  responsibilities: [
    'Measure every cost-bearing consumption per category (AI, images, email, files, simulations, exports, queues, retries, traffic, storage, CPU) from events the domains emit.',
    'Keep priority reserves: committed benefits, 30 days of operation, incidents; only available cash funds spend.',
    'Check coverage before commitments are taken on (plans, trials, promotions) and admit or refuse costly operations; concurrent work already occupies budget.',
    'Enforce monetary caps hourly, daily and monthly, global and per category, plus speed, concurrency and volume; a missing limit, zero budget or invalid cost blocks, never unlimited.',
    'Run the restriction ladder: normal, restriction (free and discretionary first), selective pause per category, automatic emergency shutdown that prevails over continuity rules.',
    'Keep the shutdown until an explicit manual reopening with second factor; reopen gradually.',
    'Keep the money picture for admin: sales, settlements, cash, contributions, growth fund, withdrawable, measured versus estimated.',
    'Split free surplus 70% growth / up to 30% voluntary withdrawal, once per surplus, with configurable parameters.'
  ],
  doesNotKnow: [
    'What the money buys for users (plan) or how it is charged (billing).',
    'The content of the operations it admits: only their category, estimate and measured cost.'
  ],
  references: ['Identity: the admin acting on limits (user identifier).'],
  sources: ['B: monetization', 'P12§1-6', 'P15§4', 'P30', 'P32']
});

// ---------- H1 (implemented by T-105): minimal core — metering, admission, restriction ladder ----------
// Reserves, cash, surplus split and the money panel need billing: they are the H6 design.

var RESTRICTION_LADDER = {
  states: ['normal', 'restricted', 'paused', 'shutdown'],
  transitions: [
    { from: 'normal', to: 'restricted', on: 'spend plus reservations press against a cap, retry loops spike, or critical cost metrics go missing' },
    { from: 'restricted', to: 'paused', on: 'one category keeps threatening its cap: that category stops, the rest continue only while controlled' },
    { from: 'restricted', to: 'normal', on: 'pressure clears (automatic)' },
    { from: 'paused', to: 'restricted', on: 'the paused category is back under its cap (automatic)' },
    { from: 'normal', to: 'shutdown', on: 'risk persists, global control fails, or the admin orders an immediate cutoff' },
    { from: 'restricted', to: 'shutdown', on: 'risk persists, global control fails, or the admin orders an immediate cutoff' },
    { from: 'paused', to: 'shutdown', on: 'risk persists, global control fails, or the admin orders an immediate cutoff' },
    { from: 'shutdown', to: 'restricted', on: 'explicit manual reopening with second factor, gradual; never a restart, deploy, month change or new revenue' }
  ]
};

PH.table({
  id: 'budget.budget_rates', domain: 'budget', status: 'implemented',
  name: 'budget_rates',
  summary: 'What a unit of each service costs Playhoot: the editable cost inputs.',
  purpose: 'Spend events carry quantities; rates turn them into USD. Registered rates also back AI credit consumption later (H6/H7). Seeded: ses-outbound 0.0001 USD/email, resend-outbound 0 (counts volume), and placeholder estimates for object storage (0.02 USD/GB-month) and compute (0.05 USD/vCPU-hour) flagged as such in their reason.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'category', type: 'text', note: 'Constant: ai, images, email, files, simulations, exports, traffic, storage, compute.' },
    { name: 'service', type: 'text', note: 'Concrete service inside the category (e.g. ses-outbound, anthropic-sonnet-input).' },
    { name: 'unit', type: 'text', note: 'What is counted (e.g. emails, tokens, GB-month).' },
    { name: 'usd_per_unit', type: 'numeric', note: '' },
    { name: 'effective_at', type: 'timestamptz', note: '' },
    { name: 'created_by_user_uuid', type: 'uuid', note: 'Admin actor; empty for the seeded rows.' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(category, service, effective_at)'],
  history: 'It is itself the history (append-only; current rate is the latest effective row per service).',
  references: [{ column: 'created_by_user_uuid', to: 'Identity: the admin who changed the rate (user identifier).' }]
});

PH.table({
  id: 'budget.budget_caps', domain: 'budget', status: 'implemented',
  name: 'budget_caps',
  summary: 'Monetary and volume ceilings per window: global and per category.',
  purpose: 'Hourly, daily and monthly USD limits plus concurrency and volume caps; a category without a current cap admits nothing. Seeded with V-04: global 1/hour, 8/day, 120/month; email 1/day.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'scope', type: 'text', note: 'global or a category constant.' },
    { name: 'cap_window', type: 'text', note: 'hour, day or month (UTC calendar windows).' },
    { name: 'usd_limit', type: 'numeric', note: 'Zero blocks the scope.' },
    { name: 'max_concurrency', type: 'int', note: 'Simultaneous admitted work; empty when not limited by count.' },
    { name: 'max_volume', type: 'int', note: 'Admitted work items per window; empty when not limited by count.' },
    { name: 'effective_at', type: 'timestamptz', note: '' },
    { name: 'created_by_user_uuid', type: 'uuid', note: 'Admin actor; empty for the seeded rows.' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(scope, cap_window, effective_at)'],
  history: 'It is itself the history (append-only; current cap is the latest effective row per scope and window).',
  references: [{ column: 'created_by_user_uuid', to: 'Identity: the admin who changed the cap (user identifier).' }]
});

PH.table({
  id: 'budget.budget_spend_events', domain: 'budget', status: 'implemented',
  name: 'budget_spend_events',
  summary: 'Every measured or estimated cost that occurred, fed by the domains through the queue.',
  purpose: 'The single spend record: windows are summed from here; compensating a user never hides a spend that happened.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'source_domain', type: 'text', note: 'Who emitted it.' },
    { name: 'category', type: 'text', note: '' },
    { name: 'service', type: 'text', note: '' },
    { name: 'reference', type: 'text', note: 'Opaque external identifier of the work (job, email, file).' },
    { name: 'quantity', type: 'numeric', note: '' },
    { name: 'unit', type: 'text', note: '' },
    { name: 'usd', type: 'numeric', note: 'quantity × registered rate, computed at ingestion.' },
    { name: 'measured', type: 'boolean', note: 'Measured usage versus estimate.' },
    { name: 'rate_missing', type: 'boolean', note: 'True when no rate existed (or its unit differed): the event is kept unpriced and the category blocks until a rate for its service exists.' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'occurred_at', type: 'timestamptz', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (source_domain, idempotency_key)', '(category, occurred_at)', '(category, reference)'],
  history: 'It is itself the history (append-only).',
  references: []
});

PH.table({
  id: 'budget.budget_reservations', domain: 'budget', status: 'implemented',
  name: 'budget_reservations',
  summary: 'Estimated cost of admitted work still in progress: it already occupies budget.',
  purpose: 'Stops many concurrent requests from spending the same budget; settled against measured cost when the work ends.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'reservation_uuid', type: 'uuid', note: 'The admission token the caller holds.' },
    { name: 'category', type: 'text', note: '' },
    { name: 'reference', type: 'text', note: 'Opaque identifier of the admitted work.' },
    { name: 'usd_reserved', type: 'numeric', note: 'The priced estimate; the margin is kept free by the admission check, not reserved.' },
    { name: 'settled_usd', type: 'numeric', note: 'Set once, from the measured spend; the remainder is released. Empty when released by expiry.' },
    { name: 'released_at', type: 'timestamptz', note: 'Set once (settled, cancelled or expired).' },
    { name: 'expires_at', type: 'timestamptz', note: 'A reservation never outlives its work window.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (reservation_uuid)', '(category, released_at)', '(category, created_at)'],
  history: 'Write-once: settled_usd, released_at.',
  references: []
});

PH.table({
  id: 'budget.budget_restriction_changes', domain: 'budget', status: 'implemented',
  name: 'budget_restriction_changes',
  summary: 'Every move of the restriction ladder; the current level is the latest row.',
  purpose: 'Normal, restricted, paused (with categories) or shutdown, with reason and actor; shutdown only leaves by a manual row.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'level', type: 'text', note: 'normal, restricted, paused or shutdown.' },
    { name: 'paused_categories', type: 'jsonb', note: 'Which categories the evaluation stopped at level paused.' },
    { name: 'reopen_closed', type: 'jsonb', note: 'Categories still closed by a gradual reopening; carried by every later row until the admin opens them.' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'actor', type: 'text', note: 'automatic, or the admin user identifier.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(created_at)'],
  history: 'It is itself the history (append-only).',
  references: [],
  lifecycle: RESTRICTION_LADDER
});

PH.operation({
  id: 'budget.record-spend', domain: 'budget', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'RecordSpend',
  summary: 'Ingests a spend event idempotently, pricing it with the registered rate.',
  reads: ['budget.budget_rates', 'budget.budget_spend_events'],
  writes: ['budget.budget_spend_events'],
  rules: [
    'Idempotent by (source domain, idempotency key): a retried task never counts twice.',
    'No registered rate: the event is stored flagged rate_missing and the category blocks new admission until an admin fixes the rates — invalid cost information never means unlimited.',
    'Measured and estimated events are distinguished and reconciled without double counting (a measured event names the reservation or estimate it replaces through its reference).',
    'The source domain is the queue the task came from, never the payload; an unreadable or invalid event turns the task dead at once (admin alert).',
    'Each new event leaves one evaluate-ladder-after-spend task (a burst collapses into one pending task).'
  ],
  enqueues: ['evaluate-ladder-after-spend'],
  sources: ['P32§1-2', 'P12§4']
});

PH.operation({
  id: 'budget.admit-work', domain: 'budget', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'AdmitWork',
  summary: 'Admits or refuses a cost-bearing operation: checks the ladder, caps and windows, then reserves the estimate.',
  reads: ['budget.budget_caps', 'budget.budget_rates', 'budget.budget_spend_events', 'budget.budget_reservations', 'budget.budget_restriction_changes'],
  writes: ['budget.budget_reservations'],
  rules: [
    'Admission counts confirmed spend, estimated not-yet-billed spend, active reservations and a margin for executing the cut (V-04: work must fit in 90% of each window remainder), against every capped window of the category and of global.',
    'One database lock serializes admissions and ladder moves, so concurrent requests never decide on the same remainder.',
    'A missing cap, a zero budget or missing rates block the category: never unlimited.',
    'Ladder gates: restricted refuses free and discretionary work first; paused refuses its categories; shutdown refuses everything.',
    'The refusal reason is a service restriction, never "your balance ran out".',
    'Concurrency and volume caps apply per category alongside the monetary ones.',
    'The reservation lives at most the declared work window (default 15 minutes, at most 24 hours); a periodic task (every minute) releases expired ones.'
  ],
  sources: ['P32§1-2', 'P32§5', 'P12§4-5']
});

PH.operation({
  id: 'budget.settle-work', domain: 'budget', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'SettleWork',
  summary: 'Closes a reservation: records the measured cost and releases the unspent remainder.',
  reads: ['budget.budget_reservations', 'budget.budget_rates'],
  writes: ['budget.budget_reservations', 'budget.budget_spend_events'],
  rules: [
    'Settling twice is idempotent (write-once settlement).',
    'Cancelled or expired work releases the whole reservation; what was actually spent still enters as a measured event.',
    'Estimates are compared with measured cost so future estimates adjust (a metric counts settlements under, equal to or over the estimate).',
    'The measured event is keyed by the reservation, so a retried settlement records it once.'
  ],
  sources: ['P12§4', 'P32§2']
});

PH.operation({
  id: 'budget.evaluate-ladder', domain: 'budget', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'EvaluateLadder',
  summary: 'Moves the restriction ladder from the numbers: escalates on pressure, de-escalates automatically below shutdown.',
  reads: ['budget.budget_caps', 'budget.budget_spend_events', 'budget.budget_reservations', 'budget.budget_restriction_changes'],
  writes: ['budget.budget_restriction_changes'],
  rules: [
    'Runs after spend ingestion and on a periodic queue task; never waits for the monthly invoice.',
    'Escalation thresholds act before a cap is reached (V-06): restricted when spend plus reservations reach 80% of any cap or the last hour’s speed projects the month cap exhausted; a category at 95% of its cap is paused; spend alone at a global cap is a failed control and shuts down.',
    'Missing or stale critical metrics restrict the affected admission automatically (H1 detects missing rates; staleness detection arrives with metered consumers).',
    'A gradual reopening in progress keeps its closed categories and the level at least restricted; de-escalation never opens them.',
    'Shutdown never lifts automatically: not by restart, deploy, month change or new revenue.',
    'Each move appends one row with reason and actor automatic.'
  ],
  steps: [
    'Sum each window per category and global: measured, estimated, reserved.',
    'Compare against current caps and escalation thresholds; detect missing metrics and rate gaps.',
    'Append the level change when it differs from the current one.',
    'Leave a notify task when the level changed (the admin learns immediately).'
  ],
  lifecycle: RESTRICTION_LADDER,
  enqueues: ['notify (level changed, keyed by the change row)', 'evaluate-ladder (periodic every minute)'],
  sources: ['P32§4-6']
});

PH.operation({
  id: 'budget.admin-set-controls', domain: 'budget', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'AdminSetControls',
  summary: 'Appends cap or rate rows with effective date, actor and reason; reached only through the step-up flow.',
  reads: ['budget.budget_caps', 'budget.budget_rates'],
  writes: ['budget.budget_caps', 'budget.budget_rates'],
  rules: [
    'Append-only: old controls stay as history.',
    'Raising caps automatically from revenue happens only with an explicit admin proportion and absolute ceiling (H6 brings the revenue side); H1 caps are manual.'
  ],
  sources: ['P32§2-3', 'P32§7', 'B: monetization']
});

PH.operation({
  id: 'budget.admin-cutoff', domain: 'budget', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'AdminCutoff',
  summary: 'Immediate manual shutdown with reason; reached only through the step-up flow.',
  reads: ['budget.budget_restriction_changes'],
  writes: ['budget.budget_restriction_changes'],
  rules: ['Appends a shutdown row with the admin as actor and a required reason; idempotent while already shut down.', 'Leaves the notify task like every ladder move.'],
  sources: ['P32§7']
});

PH.operation({
  id: 'budget.reopen', domain: 'budget', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'Reopen',
  summary: 'The only way out of shutdown: manual, second-factor verified, gradual.',
  reads: ['budget.budget_restriction_changes'],
  writes: ['budget.budget_restriction_changes'],
  rules: [
    'Reopens into restricted with every category closed (reopen_closed), then each further call opens the categories it names; the accumulated queue is never released at once.',
    'Only valid from shutdown or a reopening in progress; naming a category that is not closed is refused as already open; each step appends its row with the admin as actor.',
    'Once every category is open, the evaluation may de-escalate to normal on its own.'
  ],
  sources: ['P32§7']
});

PH.operation({
  id: 'budget.status', domain: 'budget', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'Status',
  summary: 'The control panel numbers: level, caps, window consumption measured versus estimated, reservations, spend speed and projected exhaustion.',
  reads: ['budget.budget_caps', 'budget.budget_rates', 'budget.budget_spend_events', 'budget.budget_reservations', 'budget.budget_restriction_changes'],
  writes: [],
  rules: ['Values say whether they are measured or estimated.', 'The money picture (sales, cash, reserves, surplus) arrives with H6.'],
  sources: ['P12§6', 'P32§4']
});
