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
