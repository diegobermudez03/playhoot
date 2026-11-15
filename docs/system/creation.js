// Domain: creation (planned). Detailed by the H7 design.
PH.domain({
  id: 'creation', parent: null, kind: 'traditional', status: 'planned',
  name: 'Creation',
  summary: 'The making of a game with AI: brief, conversations, jobs, validation, and external-AI access per draft.',
  responsibilities: [
    'Keep the brief of each game in creation: goal, mechanic, players, rules, ending, devices, language, grading criteria.',
    'Run the integrated AI: conversations, jobs with models and handoffs, web research for preparation, automatic fixes.',
    'Enforce the authorized maximum per job: estimate, adjustable cap, pause before exhausting it; extending is a user decision.',
    'Meter the actual AI work done (billable usage per job) and report failures that must not be charged.',
    'Keep progress recoverable and explain what was completed, checked and missing.',
    'Keep validation results against the ready criteria; finalizing stays an explicit action of the creator.',
    'Own external-AI access per draft: the read-only link, the separate modification authorization, 7-day expiry, revocation and renewal, change history with origin, idempotent resubmissions.',
    'Receive creation error reports from the user or the AI, linked to the game.'
  ],
  doesNotKnow: [
    'Draft content itself: changes, assets and simulations go to play.versions through orchestration.',
    'Credit balances (plan) and money (billing); it reports usage and reserves through orchestration.',
    'Whether a job may run (budget admits the cost).',
    'Publication (game and moderation).'
  ],
  references: [
    'Game: the game being created (game identifier).',
    'Play: the draft version being edited (version identifier).',
    'Identity: the creator and authorized AI clients (user identifier, client identifier).',
    'Library: assets it offers the AI (asset identifiers).'
  ],
  sources: ['B: create game', 'P09', 'P10', 'P12§8-10', 'P15§1', 'P24', 'P37']
});
