// Domain: notifications. Core built in T-106 (H1).
PH.domain({
  id: 'notifications', parent: null, kind: 'traditional', status: 'implemented',
  name: 'Notifications',
  summary: 'Tells users what happened: in-app entries and email, from skeletons the owning domains enqueue.',
  responsibilities: [
    'Receive one logical entry per event (external token, idempotency token, domain, type, send_at, methods, user); retries never duplicate.',
    'Render the assemblable skeleton per channel; phase 1 channels are in-app and email, never push.',
    'Keep sending separate from reading: an entry is read on opening its content, seeing the update, or explicitly; opening the list marks nothing.',
    'Keep the history with filters by domain, type and date.',
    'Keep preferences: optional and marketing independently; necessary notifications always on.',
    'Send email with minimal information, in the user\'s language, without tracking opens or clicks.',
    'Drive the outbound providers (Resend and SES behind one port) with admin modes, quota fallback, honored unsubscribes and bounces, and 30-day delivery records without bodies. Delivery 1 runs simulated local providers with the real contract; the real HTTP clients are delivery 2.'
  ],
  doesNotKnow: [
    'What an external token means: the caller owns the content and composer resolves the entity and builds the destination URL.',
    'Grades, student names, materials or secrets: they never enter an email.'
  ],
  references: ['Identity: the recipient (user UUID), their address and language, and who the administrator is (read by the orchestrator).', 'Compliance: the marketing acceptance (read by the orchestrator).', 'Budget: email admission and spend events.', 'Any domain: the entity a notification points at (opaque external token).'],
  sources: ['B: notifications', 'P14§4-6', 'P29', 'P31']
});

// ---------- Core (T-106) ----------

var EMAIL_DELIVERY_LIFECYCLE = {
  states: ['pending', 'sent', 'rejected', 'failed'],
  transitions: [
    { from: 'pending', to: 'sent', on: 'a provider accepted the message; the email is closed and never resent by either provider' },
    { from: 'pending', to: 'rejected', on: 'the provider refused it (a clear Resend quota refusal in Automatic mode leaves the email open and goes to SES), or it was rejected before any provider (provider none: suppressed address or no address)' },
    { from: 'pending', to: 'failed', on: 'a clear provider failure (the email stays open; the task retries later) or an uncertain answer or an interrupted attempt (the email is closed, never resent)' }
  ]
};

PH.table({
  id: 'notifications.notifications_entries', domain: 'notifications', status: 'verified',
  name: 'notifications_entries',
  summary: 'One logical entry per event: the skeleton, its channels and whether it was read.',
  purpose: 'Retries never duplicate entries or counters; sending is separate from reading.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'entry_uuid', type: 'uuid', note: 'External identifier.' },
    { name: 'user_uuid', type: 'uuid', note: 'Recipient; empty for an access-code entry, whose recipient is the email-code attempt identity resolves (H3, planned).' },
    { name: 'source_domain', type: 'text', note: 'The queue the notify task came from.' },
    { name: 'type', type: 'text', note: 'Constant from the P31§4 matrix.' },
    { name: 'external_token', type: 'text', note: 'Opaque entity reference; composer resolves it and builds the URL.' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'skeleton', type: 'jsonb', note: 'Assemblable content (title, paragraphs, values); the logical content belongs to the source domain. Emptied once for a type not shown in Playhoot (access code) when its email is decided.' },
    { name: 'send_at', type: 'timestamptz', note: 'When to deliver; now when past.' },
    { name: 'in_app', type: 'boolean', note: 'Shown in Playhoot (false only for access codes, a screen state of their own flow).' },
    { name: 'email_channel', type: 'boolean', note: 'Decided at acceptance from the matrix, the address, the student rule and preferences.' },
    { name: 'necessary', type: 'boolean', note: 'Access, security and billing entries ignore preferences and are never discretionary work.' },
    { name: 'read_at', type: 'timestamptz', note: 'Set once: on opening the content, seeing the update, or explicitly. Opening the list or sending email marks nothing.' },
    { name: 'content_cleared_at', type: 'timestamptz', note: 'Set once, when the skeleton was emptied.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (source_domain, idempotency_key)', 'UNIQUE (entry_uuid)', '(user_uuid, created_at)', 'partial (user_uuid) while unread and in-app'],
  history: 'Write-once: read_at, content_cleared_at.',
  references: [{ column: 'user_uuid', to: 'Identity: the recipient (user UUID).' }]
});

PH.table({
  id: 'notifications.notifications_preferences', domain: 'notifications', status: 'implemented',
  name: 'notifications_preferences',
  summary: 'Optional-email choices per group; the current row is the latest per user.',
  purpose: 'Optional and marketing are independent; necessary notifications cannot be turned off.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'user_uuid', type: 'uuid', note: '' },
    { name: 'ai_email', type: 'boolean', note: 'AI finished or paused; off by default.' },
    { name: 'capacity_email', type: 'boolean', note: 'Credits or capacities near exhaustion; off by default.' },
    { name: 'review_email', type: 'boolean', note: 'Discovery review results; on by default (V-07). A relevant restriction is its own necessary type.' },
    { name: 'marketing_email', type: 'boolean', note: 'Off by default; only effective with the specific marketing acceptance in compliance.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(user_uuid, created_at)'],
  history: 'It is itself the history (append-only; saving the same values appends nothing).',
  references: [{ column: 'user_uuid', to: 'Identity: the owner (user UUID).' }]
});

PH.table({
  id: 'notifications.notifications_email_deliveries', domain: 'notifications', status: 'verified',
  name: 'notifications_email_deliveries',
  summary: 'Delivery record of each email attempt: provider, state, never bodies, addresses or codes.',
  purpose: 'Kept 30 days; admin distinguishes pending, sent, rejected and failed.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'entry_id', type: 'bigint', note: '' },
    { name: 'provider', type: 'text', note: 'resend, ses, or none for a rejection decided before any provider.' },
    { name: 'state', type: 'text', note: 'pending, sent, rejected or failed.' },
    { name: 'provider_message_id', type: 'text', note: '' },
    { name: 'detail', type: 'text', note: 'Failure or rejection reason from a fixed list; never content.' },
    { name: 'closes_email', type: 'boolean', note: 'The attempt ended the entry email: no later attempt by either provider.' },
    { name: 'decided_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'created_at', type: 'timestamptz', note: 'Rows older than 30 days are deleted.' }
  ],
  indexes: ['(entry_id)', '(created_at)'],
  history: 'Write-once state transitions; the table itself expires at 30 days (P31§5).',
  references: [{ column: 'entry_id', to: 'notifications.notifications_entries' }],
  lifecycle: EMAIL_DELIVERY_LIFECYCLE
});

PH.table({
  id: 'notifications.notifications_suppressions', domain: 'notifications', status: 'verified',
  name: 'notifications_suppressions',
  summary: 'Addresses that must not be emailed: unsubscribes, bounces, complaints.',
  purpose: 'Honored on both providers; switching providers never evades a suppression.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'email', type: 'text', note: 'Lower-cased and trimmed.' },
    { name: 'reason', type: 'text', note: 'unsubscribe (stops optional email), bounce or complaint (stop all email).' },
    { name: 'origin', type: 'text', note: 'resend, ses or playhoot (own unsubscribe link).' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(email)'],
  history: 'It is itself the history (append-only; one row per email, reason and origin).',
  references: []
});

PH.table({
  id: 'notifications.notifications_provider_settings', domain: 'notifications', status: 'implemented',
  name: 'notifications_provider_settings',
  summary: 'The outbound email mode: Automatic, Resend only or SES only; current is the latest row.',
  purpose: 'Changed during operation without deploying, with actor and reason. Seeded at automatic.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'mode', type: 'text', note: 'automatic, resend-only or ses-only.' },
    { name: 'changed_by_user_uuid', type: 'uuid', note: 'Admin actor; empty for the initial row.' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(created_at)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'changed_by_user_uuid', to: 'Identity: the admin who changed the mode (user UUID).' }]
});

PH.table({
  id: 'notifications.notifications_provider_usage', domain: 'notifications', status: 'verified',
  name: 'notifications_provider_usage',
  summary: 'Joint daily send counters per provider account; the month is the sum of its days.',
  purpose: 'Resend respects 100/day and 3,000/month across the whole account, never multiplied by domain, instance or key.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'provider', type: 'text', note: '' },
    { name: 'day', type: 'date', note: 'UTC.' },
    { name: 'sent', type: 'int', note: 'Incremented per accepted send.' }
  ],
  indexes: ['UNIQUE (provider, day)'],
  history: 'infrastructure',
  references: []
});

PH.operation({
  id: 'notifications.accept-entry', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'verified',
  name: 'AcceptEntry',
  summary: 'Accepts one logical entry per event and decides its channels; enqueues its email task when email applies.',
  reads: ['notifications.notifications_entries', 'notifications.notifications_preferences'],
  writes: ['notifications.notifications_entries'],
  rules: [
    'Idempotent by (source domain, idempotency key): a retried task returns the existing entry.',
    'Channels come from the P31§4 matrix (type constants: in-app flag, email rule necessary | standard | optional with its group, no-email-to-students flag): no address means no email; a student under a class type gets no email; necessary and standard types keep email; optional types need their group on, and marketing also the current marketing acceptance. Preferences never trim the in-app entry.',
    'No push of any kind: in-app and email are the only channels (P29). An unknown type, an invalid recipient or a skeleton over 8 KB is invalid.',
    'The skeleton is stored as given; this domain never interprets the content.'
  ],
  enqueues: ['send-email (at send_at, keyed by the entry, when the email channel applies)'],
  sources: ['B: notifications', 'P29§1', 'P31§4']
});

PH.operation({
  id: 'notifications.send-email', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'verified',
  name: 'SendEmail',
  summary: 'Sends one entry email: picks the provider by mode and quota, renders minimal content in the user language, records each attempt.',
  reads: ['notifications.notifications_entries', 'notifications.notifications_provider_settings', 'notifications.notifications_provider_usage', 'notifications.notifications_suppressions', 'notifications.notifications_email_deliveries'],
  writes: ['notifications.notifications_email_deliveries', 'notifications.notifications_provider_usage', 'notifications.notifications_entries'],
  rules: [
    'Automatic mode uses Resend while its counted daily and monthly quota remain, then SES; a renewed quota returns to Resend. Resend only waits beyond its quota (the task retries; it never silently switches). SES only is always SES.',
    'Only a clear quota refusal by Resend in Automatic mode goes to SES in the same run; any other refusal is final; a clear failure retries later with the same rules; an uncertain answer, or an attempt left pending by a crash, closes the email: never resent by either provider. Once a provider accepted, nothing is resent.',
    'A bounce or complaint for the address rejects every email before any provider is called; an unsubscribe rejects optional email only. Priority: necessary email is never discretionary work for the budget.',
    'Minimal plain text in es/en/pt (default V-07): a fixed subject per type, a link to the notifications page, only the skeleton values the type allows (access code, budget level), the unsubscribe link only for optional types; no HTML, no open or click tracking.',
    'An accepted email increments the provider day counter and leaves a measured spend-event task (category email, service <provider>-outbound, one email) in the same transaction.',
    'Records keep provider, state, message id and a fixed detail, never bodies, addresses or codes; an access code entry keeps no content once its email is decided. A failed email never shows as delivered or read.',
    'Providers sit behind one outbound port (send, verify webhook, reference price); delivery 1 runs simulated Resend (100/day, 3,000/month joint quota, clear quota refusals) and SES (always accepts, USD 0.0001 per email) with injectable failures and uncertain answers.'
  ],
  lifecycle: EMAIL_DELIVERY_LIFECYCLE,
  enqueues: ['spend-event (per accepted email, keyed by the delivery)'],
  sources: ['P31§1-5', 'P29§1']
});

PH.operation({
  id: 'notifications.expire-deliveries', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'ExpireDeliveries',
  summary: 'Deletes email delivery records older than 30 days.',
  reads: ['notifications.notifications_email_deliveries'],
  writes: ['notifications.notifications_email_deliveries'],
  rules: ['Periodic task of the notifications queue, every hour, 500 rows per statement.'],
  enqueues: ['expire-email-deliveries (periodic every hour)'],
  sources: ['P31§5']
});

PH.operation({
  id: 'notifications.mark-read', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'verified',
  name: 'MarkRead',
  summary: 'Marks an entry read: on opening its content, seeing the update, or explicitly.',
  reads: ['notifications.notifications_entries'],
  writes: ['notifications.notifications_entries'],
  rules: ['Opening the list marks nothing; sending an email marks nothing.', 'Idempotent (write-once read_at); another user entry is unknown.'],
  sources: ['P14§4']
});

PH.operation({
  id: 'notifications.list-entries', domain: 'notifications', kind: 'usecase', effect: 'read', status: 'verified',
  name: 'ListEntries',
  summary: 'The user in-app notifications, newest first, with filters by domain, type and date, plus the unread count.',
  reads: ['notifications.notifications_entries'],
  writes: [],
  rules: ['Composer joins entries with their entities and builds destination URLs (H2+); this operation only returns the entries.', 'Pages of at most 100 (30 by default), paged with before.'],
  sources: ['P14§4-5']
});

PH.operation({
  id: 'notifications.read-preferences', domain: 'notifications', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'ReadPreferences',
  summary: 'The user current optional email choices, or the defaults.',
  reads: ['notifications.notifications_preferences'],
  writes: [],
  rules: ['Defaults until saved: AI and capacity off, review on, marketing off (V-07).'],
  sources: ['P31§4']
});

PH.operation({
  id: 'notifications.set-preferences', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'SetPreferences',
  summary: 'Appends a new preference row for the user.',
  reads: ['notifications.notifications_preferences'],
  writes: ['notifications.notifications_preferences'],
  rules: ['Necessary notifications have no switch.', 'Marketing email additionally requires the compliance marketing acceptance, checked per notice at acceptance.'],
  sources: ['P31§4', 'P22§3']
});

PH.operation({
  id: 'notifications.delete-user-entries', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'DeleteUserEntries',
  summary: 'Deletes a deleted user\'s entries and preferences; a step of the account-cleanup of H3 (D-036).',
  reads: ['notifications.notifications_entries', 'notifications.notifications_preferences'],
  writes: ['notifications.notifications_entries', 'notifications.notifications_preferences'],
  rules: [
    'Entries and preferences go; suppressions stay — they are keyed by the address and protect it, not the account (P31§3).',
    'Delivery records keep expiring on their own 30-day clock (P31§5); nothing here waits for them.',
    'Idempotent: a second run deletes nothing new.'
  ],
  sources: ['P02§11', 'P31§3-5']
});

PH.operation({
  id: 'notifications.record-suppression', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'verified',
  name: 'RecordSuppression',
  summary: 'Records an unsubscribe from the own link, or bounces and complaints from a provider verified webhook.',
  reads: ['notifications.notifications_suppressions'],
  writes: ['notifications.notifications_suppressions'],
  rules: [
    'Idempotent per (email, reason, origin).',
    'The unsubscribe token seals the address with AES-GCM: it identifies the address without exposing it; an unreadable token is refused.',
    'A webhook is verified by its provider behind the port (delivery 1: HMAC signature of the simulated provider); a rejected webhook records nothing.'
  ],
  sources: ['P31§3']
});

PH.operation({
  id: 'notifications.admin-email-status', domain: 'notifications', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'AdminEmailStatus',
  summary: 'Mode, consumption today and this month per provider, estimated cost, and the last 30 days of deliveries by state.',
  reads: ['notifications.notifications_provider_settings', 'notifications.notifications_provider_usage', 'notifications.notifications_email_deliveries'],
  writes: [],
  rules: ['Estimated cost is the month accepted sends times the provider reference price.'],
  sources: ['P31§2', 'P31§5']
});

PH.operation({
  id: 'notifications.admin-set-email-mode', domain: 'notifications', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'AdminSetEmailMode',
  summary: 'Switches between Automatic, Resend only and SES only without deploying.',
  reads: ['notifications.notifications_provider_settings'],
  writes: ['notifications.notifications_provider_settings'],
  rules: ['Needs a reason; choosing the current mode appends nothing.', 'Resend only uses only its available quota (sends beyond it wait, they never silently switch).'],
  sources: ['P31§2']
});
