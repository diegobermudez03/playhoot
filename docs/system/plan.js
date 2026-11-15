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
