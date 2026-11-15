// Domain: identity. Operation and table detail is loaded by T-004 (existing) and milestone designs (rest).
PH.domain({
  id: 'identity', parent: null, kind: 'traditional', status: 'implemented',
  name: 'Identity',
  summary: 'Who a person is and how they get in: guests, accounts, student and admin credentials, and the general permission to call each operation.',
  responsibilities: [
    'Give every person a stable user identifier that survives upgrades.',
    'Create guests (display name only, tied to the device, expire after 90 days of inactivity).',
    'Create and recover registered accounts: Google and one-time email codes; link methods; no own passwords.',
    'Upgrade a guest to a new account keeping identity and history; never merge into an existing account.',
    'Issue, renew and recognize credentials, including per-mode tokens (normal, teacher, student, admin).',
    'Issue and revoke the individual per-class student credential, and protect the adult account behind student mode.',
    'Hold the single administrator account with its second factor.',
    'Decide the general permission for each operation (account type, role, mode); limit display-name changes per window.',
    'Authenticate external AI clients (OAuth for MCP).'
  ],
  doesNotKnow: [
    'Whether a student belongs to a class or may join an activity (classroom).',
    'What a plan entitles the account to (plan); switching to teacher mode asks plan through orchestration.',
    'Age, country and acceptance evidence (compliance).',
    'Games, sessions and any resource-level scoping (each owning domain).'
  ],
  references: [
    'No foreign domain data: it is the source of the user identifier the rest of the system stores.'
  ],
  sources: ['B: identity', 'P03', 'P02§9-10']
});
