// Domain: notifications (planned). Detailed by the H1 design.
PH.domain({
  id: 'notifications', parent: null, kind: 'traditional', status: 'planned',
  name: 'Notifications',
  summary: 'Tells users what happened: in-app entries and email, from skeletons the owning domains enqueue.',
  responsibilities: [
    'Receive one logical entry per event (external token, idempotency token, domain, type, send_at, methods, user); retries never duplicate.',
    'Render the assemblable skeleton per channel; phase 1 channels are in-app and email, never push.',
    'Keep sending separate from reading: an entry is read on opening its content, seeing the update, or explicitly; opening the list marks nothing.',
    'Keep the history with filters by domain, type and date.',
    'Keep preferences: optional and marketing independently; necessary notifications always on.',
    'Send email with minimal information, in the user\'s language, without tracking opens or clicks.',
    'Drive the outbound providers (Resend and SES behind ports) with admin modes, quota fallback, honored unsubscribes and bounces, and 30-day delivery records without bodies.'
  ],
  doesNotKnow: [
    'What an external token means: the caller owns the content and composer resolves the entity and builds the destination URL.',
    'Grades, student names, materials or secrets: they never enter an email.'
  ],
  references: ['Identity: the recipient (user identifier).', 'Any domain: the entity a notification points at (opaque external token).'],
  sources: ['B: notifications', 'P14§4-6', 'P29', 'P31']
});

// ---------- H1 design (planned) ----------

var EMAIL_DELIVERY_LIFECYCLE = {
  states: ['pending', 'sent', 'rejected', 'failed'],
  transitions: [
    { from: 'pending', to: 'sent', on: 'a provider accepted the message (accepted by one is never resent by the other)' },
    { from: 'pending', to: 'rejected', on: 'the address is suppressed, or both providers refused it' },
    { from: 'pending', to: 'failed', on: 'retries exhausted or an uncertain response that was not retried immediately' }
  ]
};

PH.table({
  id: 'notifications.notifications_entries', domain: 'notifications', status: 'planned',
  name: 'notifications_entries',
  summary: 'One logical entry per event: the skeleton, its channels and whether it was read.',
  purpose: 'Retries never duplicate entries or counters; sending is separate from reading.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'entry_uuid', type: 'uuid', note: 'External identifier.' },
    { name: 'user_id', type: 'bigint', note: 'Recipient.' },
    { name: 'source_domain', type: 'text', note: 'Who emitted the event.' },
    { name: 'type', type: 'text', note: 'Constant from the P31§4 matrix.' },
    { name: 'external_token', type: 'text', note: 'Opaque entity reference; composer resolves it and builds the URL.' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'skeleton', type: 'jsonb', note: 'Assemblable content (title, paragraphs, values); the logical content belongs to the source domain.' },
    { name: 'send_at', type: 'timestamptz', note: 'When to deliver; immediate when past.' },
    { name: 'email_channel', type: 'boolean', note: 'Decided at acceptance from the matrix and preferences.' },
    { name: 'necessary', type: 'boolean', note: 'Access, security and billing entries ignore preferences.' },
    { name: 'read_at', type: 'timestamptz', note: 'Set once: on opening the content, seeing the update, or explicitly. Opening the list marks nothing.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (source_domain, idempotency_key)', 'UNIQUE (entry_uuid)', '(user_id, created_at)', 'partial (user_id) while unread'],
  history: 'Write-once: read_at.',
  references: [{ column: 'user_id', to: 'Identity: the recipient (user identifier).' }]
});

PH.table({
  id: 'notifications.notifications_preferences', domain: 'notifications', status: 'planned',
  name: 'notifications_preferences',
  summary: 'Optional-email choices per group; the current row is the latest per user.',
  purpose: 'Optional and marketing are independent; necessary notifications cannot be turned off.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'ai_email', type: 'boolean', note: 'AI finished or paused; off by default.' },
    { name: 'capacity_email', type: 'boolean', note: 'Credits or capacities near exhaustion; off by default.' },
    { name: 'review_email', type: 'boolean', note: 'Discovery review results; a relevant restriction is sent anyway (necessary).' },
    { name: 'marketing_email', type: 'boolean', note: 'Only effective with the specific marketing acceptance in compliance.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(user_id, created_at)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'user_id', to: 'Identity: the owner (user identifier).' }]
});

PH.table({
  id: 'notifications.notifications_email_deliveries', domain: 'notifications', status: 'planned',
  name: 'notifications_email_deliveries',
  summary: 'Delivery record of each email attempt: provider, state, never bodies or codes.',
  purpose: 'Kept 30 days; admin distinguishes pending, sent, rejected and failed.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'entry_id', type: 'bigint', note: '' },
    { name: 'provider', type: 'text', note: 'resend or ses.' },
    { name: 'state', type: 'text', note: 'pending, sent, rejected or failed.' },
    { name: 'provider_message_id', type: 'text', note: '' },
    { name: 'detail', type: 'text', note: 'Failure or rejection reason; never content.' },
    { name: 'decided_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: 'Rows older than 30 days are deleted.' }
  ],
  indexes: ['(entry_id)', '(created_at)'],
  history: 'Write-once state transitions; the table itself expires at 30 days (P31§5).',
  references: [{ column: 'entry_id', to: 'notifications.notifications_entries' }],
  lifecycle: EMAIL_DELIVERY_LIFECYCLE
});

PH.table({
  id: 'notifications.notifications_suppressions', domain: 'notifications', status: 'planned',
  name: 'notifications_suppressions',
  summary: 'Addresses that must not be emailed: unsubscribes, bounces, complaints.',
  purpose: 'Honored on both providers; switching providers never evades a suppression.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'email', type: 'text', note: '' },
    { name: 'reason', type: 'text', note: 'unsubscribe, bounce or complaint.' },
    { name: 'origin', type: 'text', note: 'resend, ses or playhoot (own unsubscribe link).' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(email)'],
  history: 'It is itself the history (append-only).',
  references: []
});

PH.table({
  id: 'notifications.notifications_provider_settings', domain: 'notifications', status: 'planned',
  name: 'notifications_provider_settings',
  summary: 'The outbound email mode: Automatic, Resend only or SES only; current is the latest row.',
  purpose: 'Changed during operation without deploying, with actor and reason.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'mode', type: 'text', note: 'automatic, resend-only or ses-only.' },
    { name: 'changed_by', type: 'bigint', note: 'Admin actor.' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(created_at)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'changed_by', to: 'Identity: the admin who changed the mode (user identifier).' }]
});

PH.table({
  id: 'notifications.notifications_provider_usage', domain: 'notifications', status: 'planned',
  name: 'notifications_provider_usage',
  summary: 'Joint daily and monthly send counters per provider account.',
  purpose: 'Resend respects 100/day and 3,000/month across the whole account, never multiplied by domain, instance or key.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'provider', type: 'text', note: '' },
    { name: 'day', type: 'date', note: '' },
    { name: 'sent', type: 'int', note: 'Incremented per accepted send.' }
  ],
  indexes: ['UNIQUE (provider, day)'],
  history: 'infrastructure',
  references: []
});

PH.operation({
  id: 'notifications.accept-entry', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'AcceptEntry',
  summary: 'Accepts one logical entry per event and decides its channels; enqueues its email task when email applies.',
  reads: ['notifications.notifications_entries', 'notifications.notifications_preferences'],
  writes: ['notifications.notifications_entries'],
  rules: [
    'Idempotent by (source domain, idempotency key): a retried task returns the existing entry.',
    'Channels come from the P31§4 matrix plus the recipient’s preferences; necessary types always keep both channels; students never get email.',
    'No push of any kind: in-app and email are the only channels (P29).',
    'The skeleton is stored as given; this domain never interprets the content.'
  ],
  enqueues: ['send-email (at send_at, when the email channel applies)'],
  sources: ['B: notifications', 'P29§1', 'P31§4']
});

PH.operation({
  id: 'notifications.send-email', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'SendEmail',
  summary: 'Sends one entry’s email: picks the provider by mode and quota, renders minimal content in the user’s language, records the delivery.',
  reads: ['notifications.notifications_entries', 'notifications.notifications_provider_settings', 'notifications.notifications_provider_usage', 'notifications.notifications_suppressions', 'notifications.notifications_email_deliveries'],
  writes: ['notifications.notifications_email_deliveries', 'notifications.notifications_provider_usage'],
  rules: [
    'Automatic mode uses Resend while daily and monthly quota remain, then SES; a renewed quota returns to Resend.',
    'A clear quota rejection may go to SES; once a provider accepted, the other never resends; an uncertain response is not immediately resent.',
    'Suppressed addresses are rejected before any provider is called; access, security and billing emails go before optional ones.',
    'Minimal information: links to protected content, no grades, student names, materials or secrets; no open or click tracking.',
    'Occasional losses are acceptable; the in-app entry remains and a failed email never shows as delivered or read.',
    'Providers sit behind a port: delivery 1 runs a simulated local sender with the same contract (quota, rejections, suppressions).'
  ],
  lifecycle: EMAIL_DELIVERY_LIFECYCLE,
  sources: ['P31§1-5', 'P29§1']
});

PH.operation({
  id: 'notifications.mark-read', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'MarkRead',
  summary: 'Marks an entry read: on opening its content, seeing the update, or explicitly.',
  reads: ['notifications.notifications_entries'],
  writes: ['notifications.notifications_entries'],
  rules: ['Opening the list marks nothing; sending an email marks nothing.', 'Idempotent (write-once read_at).'],
  sources: ['P14§4']
});

PH.operation({
  id: 'notifications.list-entries', domain: 'notifications', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'ListEntries',
  summary: 'The user’s notifications with filters by domain, type and date, plus the unread count.',
  reads: ['notifications.notifications_entries'],
  writes: [],
  rules: ['Composer joins entries with their entities and builds destination URLs; this operation only returns the entries.'],
  sources: ['P14§4-5']
});

PH.operation({
  id: 'notifications.set-preferences', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'SetPreferences',
  summary: 'Appends a new preference row for the user.',
  reads: ['notifications.notifications_preferences'],
  writes: ['notifications.notifications_preferences'],
  rules: ['Necessary notifications cannot be disabled.', 'Marketing email additionally requires the compliance marketing acceptance.'],
  sources: ['P31§4', 'P22§3']
});

PH.operation({
  id: 'notifications.record-suppression', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'RecordSuppression',
  summary: 'Records an unsubscribe, bounce or complaint from either provider or the own unsubscribe link.',
  reads: ['notifications.notifications_suppressions'],
  writes: ['notifications.notifications_suppressions'],
  rules: ['Idempotent per (email, reason, origin) repetition.', 'An unsubscribe token identifies the address without exposing it.'],
  sources: ['P31§3']
});

PH.operation({
  id: 'notifications.admin-email-status', domain: 'notifications', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'AdminEmailStatus',
  summary: 'Mode, consumption per provider, deliveries by state and estimated cost.',
  reads: ['notifications.notifications_provider_settings', 'notifications.notifications_provider_usage', 'notifications.notifications_email_deliveries'],
  writes: [],
  rules: [],
  sources: ['P31§2', 'P31§5']
});

PH.operation({
  id: 'notifications.admin-set-email-mode', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'AdminSetEmailMode',
  summary: 'Switches between Automatic, Resend only and SES only without deploying.',
  reads: ['notifications.notifications_provider_settings'],
  writes: ['notifications.notifications_provider_settings'],
  rules: ['Resend only uses only its available quota (sends beyond it wait or fail, they never silently switch).'],
  sources: ['P31§2']
});
