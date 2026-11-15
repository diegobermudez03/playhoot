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
