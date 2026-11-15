// Domain: billing (planned). Detailed by the H6 design.
PH.domain({
  id: 'billing', parent: null, kind: 'traditional', status: 'planned',
  name: 'Billing',
  summary: 'Money with the payment provider: purchases, charges, renewals, refunds, disputes and reconciliation.',
  responsibilities: [
    'Run checkout with the provider (Lemon Squeezy behind a port) and keep each payment pending, confirmed or rejected.',
    'Activate benefits only on reliable provider confirmation, idempotently; a confirmed payment with failed activation recovers on its own.',
    'Run renewals, the 7-day failed-renewal recovery, cancellations effective at period end, and prorated upgrades.',
    'Apply the first-subscription discount from the consumed part of paid purchases, up to two monthly fees.',
    'Handle refunds (14-day first fee, duplicates, unconsumed purchases) with request and tracking in "My purchases".',
    'Handle disputes: suspend the payment\'s benefits without banning or deleting; keep disputed uses reserved.',
    'Reconcile sales, settlements and payouts with the provider; payment confirmed, capacity enabled and spend incurred stay distinct, reconcilable facts.'
  ],
  doesNotKnow: [
    'What a benefit entitles (plan).',
    'Playhoot\'s own costs and reserves (budget).',
    'Card or account data beyond what the Merchant of Record returns.'
  ],
  references: [
    'Identity: the paying account (user identifier).',
    'Plan: the catalog item or pool each purchase buys (plan identifiers).'
  ],
  sources: ['P11', 'P15§4', 'P23', 'P26']
});
