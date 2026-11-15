// Domain: library (planned). Detailed by the H7 design.
PH.domain({
  id: 'library', parent: null, kind: 'traditional', status: 'planned',
  name: 'Library',
  summary: 'The platform asset library: records, versions, states, licenses and variants the AI and creators draw from.',
  responsibilities: [
    'Keep every platform asset (SVG, Lottie, images, sounds) with its state: available, pending or retired.',
    'Keep pending items with what the operator must supply; never let a pending asset be used as if it existed.',
    'Answer search by function and style, with preview and listen.',
    'Version assets: an update is a new version for future uses; it never changes existing games, Sessions or replays.',
    'Keep provenance, license and attributions of every asset, including external sources (Kenney, Lucide, hosted Google Fonts).',
    'Register derived variants the AI produces, with reference to origin and version; adding one to the common catalog is an admin decision.'
  ],
  doesNotKnow: [
    'Which games use an asset (versions copy what they use).',
    'AI jobs and their costs (creation).'
  ],
  references: ['Identity: who contributed or approved an asset (user identifier).'],
  sources: ['P25', 'P09§9', 'P36§5', 'P04§2']
});
