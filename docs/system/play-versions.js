// Subdomain: play.versions (planned split out of play/session). Detailed by the H4 design.
PH.domain({
  id: 'play.versions', parent: 'play', kind: 'traditional', status: 'planned',
  name: 'Play versions',
  summary: 'The content of each game version: scripts, assets, builds, previews, and which version is in which condition.',
  responsibilities: [
    'Keep drafts with revisions, diffs, restore and optimistic conflict detection.',
    'Build versions, run their checks and keep diagnostics.',
    'Freeze a version when it becomes ready, assets included; any later change prepares a new version.',
    'Distinguish the four version conditions: in editing, ready and chosen for new Sessions, approved for Discovery, pinned in an existing Session.',
    'Allow returning to a previous available and authorized version.',
    'Keep simulation captures and the screenshots offered with a version.',
    'Keep what a replay needs from its original version through the replay retention window.'
  ],
  doesNotKnow: [
    'Who owns the game or whether it may be published (game).',
    'Review decisions (moderation).',
    'The creation conversation that produced a change (creation sends changes through orchestration).'
  ],
  references: [
    'Game: the logical game of each version (game identifier).',
    'Library: platform assets included in a version (asset identifiers).'
  ],
  sources: ['B: play', 'P04', 'P10§5', 'P15§2']
});
