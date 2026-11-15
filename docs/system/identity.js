// Domain: identity. Detail is loaded by a later task.
PH.domain({
  id: 'identity', parent: null, kind: 'traditional', status: 'implemented',
  name: 'Identity',
  summary: 'Who a person is: guests, accounts, credentials, and access levels per operation.',
  responsibilities: [
    'Give every person a stable user identifier.',
    'Issue and renew credentials.',
    'Decide the minimum access level of each operation.'
  ],
  doesNotKnow: ['Games and Sessions.'],
  sources: []
});
