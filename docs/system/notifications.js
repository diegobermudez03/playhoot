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
