// Domain: plan (planned). Detailed by the H6 design.
PH.domain({
  id: 'plan', parent: null, kind: 'traditional', status: 'planned',
  name: 'Plan',
  summary: 'What each account is entitled to: the plan catalog, capacities, usable purchases and commercial balances, including AI credits.',
  responsibilities: [
    'Keep the plan catalog: families, tiers, prices, allowances, features, validity and history, manageable from admin with drafts.',
    'Apply price protection: increases only for new subscriptions; reductions at the next charge without exceeding the protected rate.',
    'Keep configurable free limits with effective date, simulation and history; lowering them creates no debt and deletes nothing.',
    'Answer the capacity limits of any user (free defaults when there is no record); the caller passes them to the owning domain.',
    'Keep entitlements current: holding a teacher token does not keep paid functions after expiry.',
    'Keep AI credit balances: validity, consumption order, reservation per job, idempotent deduction, compensation when Playhoot loses the work.',
    'Keep one-off purchase pools (sessions, creations, players, top-ups) with their validity.',
    'Reserve a session use at lobby, consume it at start, release or restore it when the rules say so.',
    'Run the teacher trial pools: independent, expiring at the end, larger capacity applies when a Personal plan exists.'
  ],
  doesNotKnow: [
    'Charging money: billing confirms payments; plan enables what was bought.',
    'Actual spend and whether Playhoot can afford a commitment (budget).',
    'Actual usage counts: game knows creations, play knows sessions, creation knows AI work.'
  ],
  references: [
    'Identity: the account each entitlement belongs to (user identifier).',
    'Billing: the purchase that enabled a benefit (payment identifier).'
  ],
  sources: ['B: plan', 'P11', 'P12§7-12', 'P15§4-5', 'P26', 'P27', 'P28']
});

// ---------- H1 design (planned): minimal core — configurable free limits ----------
// The catalog, subscriptions, purchases, credits and trial pools are the H6 design.

PH.table({
  id: 'plan.plan_free_limit_configs', domain: 'plan', status: 'planned',
  name: 'plan_free_limit_configs',
  summary: 'The free capacities every account without a subscription gets, configurable with an effective date.',
  purpose: 'Seeded with P27§2: 3 saved games, 2 creations and 5 sessions per rolling 30 days, 10 players, 1 room, 100 MB.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'saved_games', type: 'int', note: 'Includes drafts.' },
    { name: 'creations_per_window', type: 'int', note: '' },
    { name: 'sessions_per_window', type: 'int', note: 'Sessions started.' },
    { name: 'window_days', type: 'int', note: 'Rolling window, 30 by default.' },
    { name: 'players_per_session', type: 'int', note: '' },
    { name: 'simultaneous_rooms', type: 'int', note: 'Lobbies included.' },
    { name: 'storage_mb', type: 'int', note: '' },
    { name: 'effective_at', type: 'timestamptz', note: 'May be in the future.' },
    { name: 'created_by', type: 'bigint', note: 'Admin actor.' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(effective_at)'],
  history: 'It is itself the history (append-only; the current config is the latest effective row).',
  references: [{ column: 'created_by', to: 'Identity: the admin who changed the limits (user identifier).' }]
});

PH.operation({
  id: 'plan.get-user-limits', domain: 'plan', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'GetUserLimits',
  summary: 'The capacity limits of a user; in H1 everyone gets the current free config (subscriptions arrive with H6).',
  reads: ['plan.plan_free_limit_configs'],
  writes: [],
  rules: [
    'Free defaults when there is no plan record; never an error for an unknown user.',
    'The caller (orchestrator) passes the values to the owning domain, which verifies actual usage and applies the most restrictive of received and own (the three-level limits convention).',
    'Values sent by the browser or an AI never grant capacity.'
  ],
  sources: ['B: plan', 'P27§2', 'P15§5']
});

PH.operation({
  id: 'plan.read-free-limits', domain: 'plan', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'ReadFreeLimits',
  summary: 'The free-limit config for admin: current, scheduled and history.',
  reads: ['plan.plan_free_limit_configs'],
  writes: [],
  rules: [],
  sources: ['P27§3']
});

PH.operation({
  id: 'plan.admin-set-free-limits', domain: 'plan', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'AdminSetFreeLimits',
  summary: 'Appends a new free-limit config with an effective date, actor and reason.',
  reads: ['plan.plan_free_limit_configs'],
  writes: ['plan.plan_free_limit_configs'],
  rules: [
    'Lowering a limit creates no debt and deletes nothing: accounts over the new capacity only lose the ability to add more.',
    'The impact simulation over real usage is a composed admin view (H10).'
  ],
  sources: ['P27§3']
});
