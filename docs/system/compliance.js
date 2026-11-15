// Domain: compliance (planned). Core in the H1 design; authorizations and the guardian page in H8; privacy requests in H11.
PH.domain({
  id: 'compliance', parent: null, kind: 'traditional', status: 'planned',
  name: 'Compliance',
  summary: 'Legal rules and their evidence: markets, declared age, acceptances, minors\' authorizations, privacy requests and the provider registry.',
  responsibilities: [
    'Keep the market-and-function matrix (configurable per market and function) and answer whether a function is available to a person and place.',
    'Keep declared age and country (never "verified"), with the date, resistant to retries; corrections go through support.',
    'Apply territorial control signals at entry and prior data upload: declared residence plus approximate location, no GPS.',
    'Treat known minority as binding even against an adult declaration.',
    'Keep versioned, distinguishable acceptances: terms, own-data processing, the class data agreement (downloadable), marketing.',
    'Keep the teacher declaration before real students: own behalf or institution, confirmed contact, authority.',
    'Keep minors\' authorization coverage per student (existing evidence, batch registration) and run the guardian page for new authorizations: confirmed email, express acceptance, receipt, lookup and withdrawal the teacher cannot override.',
    'Receive privacy requests (access, correction, deletion, withdrawal), even without an account, with tracking and proportional identity verification.',
    'Keep the queryable provider registry: function, data, countries.'
  ],
  doesNotKnow: [
    'Credentials and tokens (identity).',
    'Class content and grades (classroom): it only keeps who is authorized for what.',
    'Whether to delete or substitute data in other domains: orchestration carries its decisions there.'
  ],
  references: [
    'Identity: the person each declaration, acceptance or request belongs to (user identifier).',
    'Classroom: the class or group an authorization covers (class identifier).'
  ],
  sources: ['P02§15', 'P19', 'P20', 'P21', 'P22', 'P35']
});

// ---------- H1 design (planned): core — market matrix, declarations, acceptances ----------

PH.table({
  id: 'compliance.compliance_market_rules', domain: 'compliance', status: 'planned',
  name: 'compliance_market_rules',
  summary: 'The market-and-function matrix: what each place may do and from what age.',
  purpose: 'Configurable per market and function; a function with no enabled current row is unavailable there. Seeded from P35§1-3 (US/CA per state or province; Peru present but disabled).',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'country', type: 'text', note: 'ISO 3166-1 alpha-2 constant.' },
    { name: 'subdivision', type: 'text', note: 'State or province when thresholds differ; empty means the whole country.' },
    { name: 'function', type: 'text', note: 'Constant: create-games, subscribe, teach, minor-class-participation, minor-family-participation, interactive-use.' },
    { name: 'enabled', type: 'boolean', note: '' },
    { name: 'min_age', type: 'int', note: 'Declared-age threshold for adult functions (18/19/21 per P35§3).' },
    { name: 'conditions_note', type: 'text', note: 'Pending operator conditions that keep it disabled (e.g. AAIP, Peru representation).' },
    { name: 'effective_at', type: 'timestamptz', note: '' },
    { name: 'created_by', type: 'bigint', note: 'Admin actor.' },
    { name: 'reason', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(country, subdivision, function, effective_at)'],
  history: 'It is itself the history (append-only; the current rule is the latest effective row per key).',
  references: [{ column: 'created_by', to: 'Identity: the admin who changed the matrix (user identifier).' }]
});

PH.table({
  id: 'compliance.compliance_declarations', domain: 'compliance', status: 'planned',
  name: 'compliance_declarations',
  summary: 'Declared residence and age: country, subdivision, and having reached the required age, with the date.',
  purpose: 'Stored as declared, never "verified". The current declaration is the latest row per user.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'country', type: 'text', note: '' },
    { name: 'subdivision', type: 'text', note: '' },
    { name: 'meets_age', type: 'boolean', note: 'Declared having reached the threshold.' },
    { name: 'threshold_age', type: 'int', note: 'The threshold that applied when declaring.' },
    { name: 'origin', type: 'text', note: 'user or support (corrections only via support).' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(user_id, created_at)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'user_id', to: 'Identity: the declaring person (user identifier).' }]
});

PH.table({
  id: 'compliance.compliance_minority_signals', domain: 'compliance', status: 'planned',
  name: 'compliance_minority_signals',
  summary: 'Known minority: once recorded it binds, even against an adult declaration.',
  purpose: 'Origins: a below-threshold declaration, class context, family group, a report.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'origin', type: 'text', note: 'declaration, class-context, family-group or report.' },
    { name: 'noted_by', type: 'bigint', note: 'Who or what recorded it; empty when automatic.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(user_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'user_id', to: 'Identity: the person the signal is about (user identifier).' }]
});

PH.table({
  id: 'compliance.compliance_acceptance_texts', domain: 'compliance', status: 'planned',
  name: 'compliance_acceptance_texts',
  summary: 'The versioned texts people accept: terms, own-data processing, class data agreement, marketing.',
  purpose: 'Each version per language is immutable once published; the class data agreement stays downloadable.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'kind', type: 'text', note: 'terms, own-data, class-data-agreement or marketing.' },
    { name: 'version', type: 'int', note: '' },
    { name: 'language', type: 'text', note: 'es, en or pt.' },
    { name: 'content_ref', type: 'text', note: 'Stored document (object storage), never edited after publishing.' },
    { name: 'published_at', type: 'timestamptz', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (kind, version, language)'],
  history: 'It is itself the history (append-only).',
  references: []
});

PH.table({
  id: 'compliance.compliance_acceptances', domain: 'compliance', status: 'planned',
  name: 'compliance_acceptances',
  summary: 'Who accepted which text version, and marketing withdrawals.',
  purpose: 'Distinguishable, recorded acceptances (P22§3); evidence kept with date and version.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'user_id', type: 'bigint', note: '' },
    { name: 'text_id', type: 'bigint', note: 'The exact version accepted.' },
    { name: 'action', type: 'text', note: 'accepted or withdrawn (withdrawal only for marketing).' },
    { name: 'context', type: 'text', note: 'Where it happened (registration, purchase, class setup).' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(user_id, created_at)', '(text_id)'],
  history: 'It is itself the history (append-only).',
  references: [
    { column: 'user_id', to: 'Identity: the accepting person (user identifier).' },
    { column: 'text_id', to: 'compliance.compliance_acceptance_texts' }
  ]
});

PH.operation({
  id: 'compliance.check-market-access', domain: 'compliance', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'CheckMarketAccess',
  summary: 'Answers whether a function is available to this person in this place: allowed, needs-declaration, or blocked with a reason.',
  reads: ['compliance.compliance_market_rules', 'compliance.compliance_declarations', 'compliance.compliance_minority_signals'],
  writes: [],
  rules: [
    'The current matrix row for (country, subdivision, function) decides; no row or disabled means blocked.',
    'Declared residence plus the caller-provided approximate location signal resolve discrepancies; no GPS, and travel or a VPN alone never block.',
    'A known minority signal binds even against an adult declaration.',
    'From a disabled market only public information is reachable; language never widens coverage.',
    'The refusal reason is a service restriction message, never legal advice.'
  ],
  sources: ['P35§1-5', 'P21§6', 'P19§1']
});

PH.operation({
  id: 'compliance.record-declaration', domain: 'compliance', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'RecordDeclaration',
  summary: 'Records the residence-and-age declaration, once.',
  reads: ['compliance.compliance_declarations', 'compliance.compliance_market_rules'],
  writes: ['compliance.compliance_declarations', 'compliance.compliance_minority_signals'],
  rules: [
    'Repeating the same answer is idempotent; a different answer is refused — corrections only through support (origin support).',
    'Declaring below the threshold records a binding minority signal.',
    'Google, email or card never substitute the declaration; nothing is ever marked "verified".'
  ],
  sources: ['P19§1-3', 'P35§3']
});

PH.operation({
  id: 'compliance.record-acceptance', domain: 'compliance', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'RecordAcceptance',
  summary: 'Records the acceptance of a specific text version, or a marketing withdrawal.',
  reads: ['compliance.compliance_acceptance_texts'],
  writes: ['compliance.compliance_acceptances'],
  rules: [
    'The acceptance points at the exact version shown; re-accepting the same version is idempotent.',
    'Necessary kinds (terms, own-data) cannot be withdrawn here; marketing can.',
    'Operations that require a current acceptance check for one of the latest version.'
  ],
  sources: ['P22§2-3']
});

PH.operation({
  id: 'compliance.get-acceptance-text', domain: 'compliance', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'GetAcceptanceText',
  summary: 'Returns the current version of a text kind in a language, and whether the caller already accepted it.',
  reads: ['compliance.compliance_acceptance_texts', 'compliance.compliance_acceptances'],
  writes: [],
  rules: ['The class data agreement is downloadable.'],
  sources: ['P22§2']
});

PH.operation({
  id: 'compliance.read-market-rules', domain: 'compliance', kind: 'usecase', effect: 'read', status: 'planned',
  name: 'ReadMarketRules',
  summary: 'The matrix for admin: current rules, pending conditions and history.',
  reads: ['compliance.compliance_market_rules'],
  writes: [],
  rules: [],
  sources: ['P35§1', 'P35§6']
});

PH.operation({
  id: 'compliance.admin-set-market-rule', domain: 'compliance', kind: 'usecase', effect: 'write', status: 'planned',
  name: 'AdminSetMarketRule',
  summary: 'Adds a matrix row with effective date, actor and reason; activation never precedes its operator conditions.',
  reads: ['compliance.compliance_market_rules'],
  writes: ['compliance.compliance_market_rules'],
  rules: [
    'Append-only: the old rule stays as history.',
    'Enabling a market with pending conditions noted requires clearing the note (the admin confirms the real conditions are met).'
  ],
  sources: ['P35§2', 'P35§6']
});
