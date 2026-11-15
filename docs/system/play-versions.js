// Subdomain: play.versions. Code package: play/session (versiondrafting, versionbuilding, previews).
// Status stays planned at the domain level (the four conditions and the split from play.session
// are future design); the tables and operations that already exist are loaded individually as
// implemented by T-004.
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

var BUILD_LIFECYCLE = {
  states: ['QUEUED', 'RUNNING', 'SUCCEEDED', 'FAILED', 'SUPERSEDED'],
  transitions: [
    { from: 'QUEUED', to: 'RUNNING', on: 'the build worker picks it up (FOR UPDATE SKIP LOCKED)' },
    { from: 'RUNNING', to: 'SUCCEEDED', on: 'the script check and build both pass' },
    { from: 'RUNNING', to: 'FAILED', on: 'script or build errors, or a platform failure (reported as infrastructure, never blamed on the game)' },
    { from: 'QUEUED', to: 'SUPERSEDED', on: 'a newer revision of the same version is applied' },
    { from: 'RUNNING', to: 'QUEUED', on: 'the instance restarts mid-build' }
  ]
};

PH.table({
  id: 'play.versions.play_game_version_revisions', domain: 'play.versions', status: 'implemented',
  name: 'play_game_version_revisions',
  summary: 'One source-code snapshot of a draft version.',
  purpose: 'Write-once; this IS the draft\'s history, not a separate change table.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'version_id', type: 'bigint', note: '' },
    { name: 'number', type: 'bigint', note: 'Sequential per version.' },
    { name: 'files', type: 'jsonb', note: 'Path to content-hash map.' },
    { name: 'assets', type: 'jsonb', note: 'Keys included at this revision.' },
    { name: 'kind', type: 'text', note: 'template, edition, change, restore or asset.' },
    { name: 'origin', type: 'text', note: 'web, api or mcp; empty if made by the platform.' },
    { name: 'actor', type: 'text', note: 'person, ai or platform.' },
    { name: 'parent_revision', type: 'bigint', note: '' },
    { name: 'restored_from', type: 'bigint', note: 'Set only for a restore.' },
    { name: 'sdk', type: 'text', note: 'Toolchain string recorded at this revision.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (version_id, number)'],
  history: 'Write-once: the revision itself is one change, never edited.',
  references: [{ column: 'version_id', to: 'play.session.play_game_versions' }]
});

PH.table({
  id: 'play.versions.play_source_files', domain: 'play.versions', status: 'implemented',
  name: 'play_source_files',
  summary: 'File content addressed by hash, shared across versions and games.',
  purpose: '',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'sha256', type: 'text', note: '' },
    { name: 'content', type: 'bytea', note: '' },
    { name: 'size', type: 'bigint', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (sha256)'],
  history: 'infrastructure',
  references: []
});

PH.table({
  id: 'play.versions.play_game_version_change_attempts', domain: 'play.versions', status: 'implemented',
  name: 'play_game_version_change_attempts',
  summary: 'Idempotency of a draft change: one row per version and key.',
  purpose: 'Carries the diagnostics and outcome of the attempt (applied, conflict, invalid, unchanged, not_editable).',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'version_id', type: 'bigint', note: '' },
    { name: 'idempotency_key', type: 'text', note: '' },
    { name: 'request_sha256', type: 'text', note: 'Same key with a different request is a conflict.' },
    { name: 'base_revision', type: 'bigint', note: 'Optimistic concurrency base.' },
    { name: 'origin', type: 'text', note: 'web, api or mcp.' },
    { name: 'outcome', type: 'text', note: '' },
    { name: 'diagnostics', type: 'jsonb', note: 'By path, when invalid.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (version_id, idempotency_key)', '(created_at)'],
  history: 'Write-once: claimed and completed in one transaction.',
  references: [{ column: 'version_id', to: 'play.session.play_game_versions' }]
});

PH.table({
  id: 'play.versions.play_asset_objects', domain: 'play.versions', status: 'implemented',
  name: 'play_asset_objects',
  summary: 'Registry of asset bytes, recorded before upload.',
  purpose: 'Content-addressed; media dimensions and duration are measured on upload from the file headers.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'sha256', type: 'text', note: '' },
    { name: 'mime_type', type: 'text', note: '' },
    { name: 'width, height', type: 'int, int', note: 'Empty if not an image or video.' },
    { name: 'duration_ms', type: 'bigint', note: 'Empty if not audio or video, or unknown.' },
    { name: 'deleted_at', type: 'timestamptz', note: 'Set once, by cleanup.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (sha256)', 'cleanup index on deleted_at'],
  history: 'Write-once: deleted_at.',
  references: []
});

PH.table({
  id: 'play.versions.play_game_version_assets', domain: 'play.versions', status: 'implemented',
  name: 'play_game_version_assets',
  summary: 'The assets a version uses, by logical key.',
  purpose: 'object_key empty means the asset no longer exists (removal keeps the row as history).',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'version_id', type: 'bigint', note: '' },
    { name: 'key', type: 'text', note: 'Logical name the script refers to.' },
    { name: 'type', type: 'text', note: 'image, audio or video.' },
    { name: 'mime_type', type: 'text', note: '' },
    { name: 'object_key', type: 'text', note: 'Empty once removed.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (version_id, key)'],
  history: 'Removal clears object_key rather than deleting the row.',
  references: [{ column: 'version_id', to: 'play.session.play_game_versions' }]
});

PH.table({
  id: 'play.versions.play_game_version_signed_urls', domain: 'play.versions', status: 'implemented',
  name: 'play_game_version_signed_urls',
  summary: 'The current batch of signed URLs (frontend package + assets) for a version.',
  purpose: 'Shared across Sessions and instances; reused while not within 5 minutes of expiring.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'game_version_id', type: 'bigint', note: '' },
    { name: 'urls', type: 'jsonb', note: '' },
    { name: 'expires_at', type: 'timestamptz', note: '24 h from issuing.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (game_version_id)'],
  history: 'Overwritten without history: a cache, not a fact.',
  references: [{ column: 'game_version_id', to: 'play.session.play_game_versions' }]
});

PH.table({
  id: 'play.versions.play_game_version_input_requirements', domain: 'play.versions', status: 'implemented',
  name: 'play_game_version_input_requirements',
  summary: 'What input a version\'s script declares it needs.',
  purpose: 'Produced by the build; read before a player joins from a given device.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'version_id', type: 'bigint', note: '' },
    { name: 'uses_keyboard, uses_pointer_hover, uses_pointer_press', type: 'boolean x3', note: '' },
    { name: 'devices', type: 'text', note: 'desktop, mobile (reserved) or both.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (version_id)', '(devices, version_id)'],
  history: 'infrastructure',
  references: [{ column: 'version_id', to: 'play.session.play_game_versions' }]
});

PH.table({
  id: 'play.versions.play_game_version_builds', domain: 'play.versions', status: 'implemented',
  name: 'play_game_version_builds',
  summary: 'One build attempt of one revision; doubles as the build job queue.',
  purpose: 'A successful build with the same inputs hash is reused instead of rebuilding.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'version_id', type: 'bigint', note: '' },
    { name: 'revision', type: 'bigint', note: '' },
    { name: 'status', type: 'text', note: 'See lifecycle.' },
    { name: 'inputs_sha256', type: 'text', note: 'Sources + assets + toolchain version.' },
    { name: 'toolchain_version', type: 'text', note: '' },
    { name: 'diagnostics', type: 'jsonb', note: '' },
    { name: 'script_object_key, script_sha256, script_size', type: 'text, text, bigint', note: 'Build artifacts.' },
    { name: 'frontend_object_key, frontend_sha256, frontend_size', type: 'text, text, bigint', note: 'Build artifacts.' },
    { name: 'manifest', type: 'jsonb', note: '' },
    { name: 'aspect_ratio', type: 'text', note: '' },
    { name: 'languages', type: 'jsonb', note: '' },
    { name: 'uses_keyboard, uses_pointer_hover, uses_pointer_press', type: 'boolean x3', note: '' },
    { name: 'reused_build_id', type: 'bigint', note: 'Set when this build reused a previous one\'s artifacts.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (version_id, revision)', 'partial index on status for QUEUED (the queue)', '(version_id, inputs_sha256)'],
  history: 'Change table play_game_version_build_status_changes.',
  references: [
    { column: 'version_id', to: 'play.session.play_game_versions' },
    { column: 'reused_build_id', to: 'play.versions.play_game_version_builds' }
  ],
  lifecycle: BUILD_LIFECYCLE
});

PH.table({
  id: 'play.versions.play_game_version_build_status_changes', domain: 'play.versions', status: 'implemented',
  name: 'play_game_version_build_status_changes',
  summary: 'Every status a build went through.',
  purpose: 'Append-only history.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'build_id', type: 'bigint', note: '' },
    { name: 'from_status', type: 'text', note: 'Empty for the creation.' },
    { name: 'to_status', type: 'text', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(build_id)'],
  history: 'It is itself the history (append-only).',
  references: [{ column: 'build_id', to: 'play.versions.play_game_version_builds' }]
});

PH.table({
  id: 'play.versions.play_build_artifact_objects', domain: 'play.versions', status: 'implemented',
  name: 'play_build_artifact_objects',
  summary: 'Registry of build artifact bytes, recorded before upload.',
  purpose: 'Content-addressed, same pattern as other object registries.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'sha256', type: 'text', note: '' },
    { name: 'deleted_at', type: 'timestamptz', note: 'Set once, by cleanup.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['UNIQUE (sha256)'],
  history: 'Write-once: deleted_at.',
  references: []
});

PH.table({
  id: 'play.versions.play_previews', domain: 'play.versions', status: 'implemented',
  name: 'play_previews',
  summary: 'A live preview of a successful build, without students or grades.',
  purpose: 'At most one live preview per version; opening another replaces it.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'uuid', type: 'uuid', note: '' },
    { name: 'version_id', type: 'bigint', note: '' },
    { name: 'revision', type: 'bigint', note: '' },
    { name: 'owner_user_uuid', type: 'uuid', note: '' },
    { name: 'participants', type: 'jsonb', note: '' },
    { name: 'expires_at', type: 'timestamptz', note: '10 min idle.' },
    { name: 'ended_at', type: 'timestamptz', note: 'Set once.' },
    { name: 'end_reason', type: 'text', note: 'finished, idle, ended_by_owner, replaced, script_failure or unavailable.' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['partial UNIQUE (version_id) while ended_at IS NULL'],
  history: 'Write-once: ended_at, end_reason.',
  references: [
    { column: 'version_id', to: 'play.session.play_game_versions' },
    { column: 'owner_user_uuid', to: 'identity user' }
  ]
});

PH.table({
  id: 'play.versions.play_simulation_results', domain: 'play.versions', status: 'implemented',
  name: 'play_simulation_results',
  summary: 'The result of one scenario simulation against a draft.',
  purpose: 'Readable for 30 minutes; the row is kept 7 days only to count simulations.',
  columns: [
    { name: 'id', type: 'bigserial', note: '' },
    { name: 'version_id', type: 'bigint', note: '' },
    { name: 'result', type: 'jsonb', note: '' },
    { name: 'expires_at', type: 'timestamptz', note: '' },
    { name: 'created_at', type: 'timestamptz', note: '' }
  ],
  indexes: ['(expires_at)'],
  history: 'infrastructure',
  references: [{ column: 'version_id', to: 'play.session.play_game_versions' }]
});

PH.operation({
  id: 'play.versions.create-draft-version', domain: 'play.versions', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'CreateDraftVersion',
  summary: 'Creates a DRAFT version for a game, optionally seeded from a template or a previous version.',
  reads: ['play.session.play_game_versions'],
  writes: ['play.session.play_game_versions', 'play.versions.play_game_version_revisions'],
  rules: ['A game has at most one DRAFT version at a time (partial index).'],
  sources: []
});

PH.operation({
  id: 'play.versions.apply-changes', domain: 'play.versions', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'ApplyChanges / ApplyTemplate / RestoreRevision',
  summary: 'Applies a set of file edits (find/replace with unified diff), a template, or restores a previous revision, as a new revision.',
  reads: ['play.versions.play_game_version_revisions', 'play.versions.play_game_version_change_attempts'],
  writes: [
    'play.versions.play_game_version_change_attempts', 'play.versions.play_game_version_revisions',
    'play.versions.play_source_files', 'play.versions.play_game_version_builds'
  ],
  rules: [
    'One transaction: claim the attempt by key and request hash, then decide not_editable, conflict (base revision stale), invalid (per-path diagnostics), unchanged, or applied.',
    'Applying marks the version\'s earlier QUEUED builds SUPERSEDED and enqueues a new build.'
  ],
  limits: [{ what: 'Draft sources', value: '100 files, 256 KiB per file, 2 MiB total', source: 'domain' }],
  sources: []
});

PH.operation({
  id: 'play.versions.read-draft', domain: 'play.versions', kind: 'usecase', effect: 'read', status: 'implemented',
  name: 'ReadDraft / ListRevisions / RevisionChanges / DiffRevisions / ReadBuild / LoadBuiltRevision / CheckPublication',
  summary: 'Reads of a draft\'s content, revision history, diffs, build state and publication readiness.',
  reads: ['play.versions.play_game_version_revisions', 'play.versions.play_game_version_builds', 'play.versions.play_game_version_assets'],
  writes: [],
  rules: [
    'ReadBuild may long-poll up to 20 s for a build to finish.',
    'CheckPublication\'s blockers: build_not_for_current_revision, build_not_succeeded, missing_required_assets, revision_changed.'
  ],
  sources: []
});

PH.operation({
  id: 'play.versions.upload-asset', domain: 'play.versions', kind: 'usecase', effect: 'write', status: 'implemented',
  name: 'UploadAsset / RemoveAsset',
  summary: 'Uploads or removes an asset of a draft, as a new revision.',
  reads: ['play.versions.play_asset_objects'],
  writes: ['play.versions.play_asset_objects', 'play.versions.play_game_version_assets', 'play.versions.play_game_version_revisions'],
  rules: [
    'Up to 4 uploads in memory per instance at once; a 5th waits up to 2 s (ErrUploadsBusy).',
    'Reads cut off at the type\'s size limit, hashes the bytes, and determines the real type from bytes, not the extension.'
  ],
  limits: [{ what: 'Asset size', value: 'image 5 MiB, audio 10 MiB, video 50 MiB; 100 MiB per version', source: 'domain' }],
  sources: []
});

PH.operation({
  id: 'play.versions.freeze-version', domain: 'play.versions', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'FreezeVersion / SignBuiltFrontend / CleanUnusedObjects',
  summary: 'Freezes a DRAFT into an immutable EXECUTABLE version at publication, and cleans up objects no revision references any more.',
  reads: ['play.versions.play_game_version_builds', 'play.session.play_game_versions'],
  writes: ['play.session.play_game_versions'],
  rules: [
    'Requires a SUCCEEDED build of the current revision; already_frozen is idempotent, revision_changed if the draft moved since.',
    'CleanUnusedObjects runs best-effort after a successful publish.'
  ],
  lifecycle: { states: ['DRAFT', 'EXECUTABLE'], transitions: [{ from: 'DRAFT', to: 'EXECUTABLE', on: 'FreezeVersion' }] },
  sources: []
});

PH.operation({
  id: 'play.versions.open-preview', domain: 'play.versions', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'previewlifecycle (OpenPreview / EndPreview / FindPreview)',
  summary: 'Opens a durable preview row for a successful build; play.runtime drives the live connection over it.',
  reads: ['play.versions.play_game_version_builds', 'play.versions.play_previews'],
  writes: ['play.versions.play_previews'],
  rules: ['At most one live preview per version; opening another replaces the previous one (end_reason replaced).'],
  limits: [{ what: 'Previews', value: '50 per instance, 10 min idle', source: 'domain' }],
  sources: []
});

PH.operation({
  id: 'play.versions.run-build', domain: 'play.versions', kind: 'workflow', effect: 'write', status: 'implemented',
  name: 'versionbuilding (Run / RequeueInterrupted / RunNextBuild)',
  summary: 'The build worker: picks the next QUEUED build, compiles it and records the outcome.',
  reads: ['play.versions.play_game_version_builds', 'play.versions.play_game_version_revisions'],
  writes: [
    'play.versions.play_game_version_builds', 'play.versions.play_game_version_build_status_changes',
    'play.versions.play_build_artifact_objects', 'play.versions.play_game_version_input_requirements'
  ],
  rules: [
    'One worker per instance, woken by an in-process signal and every 5 s otherwise.',
    'Validates the script against its profile (handwritten or sdk-v1) and parses the manifest.',
    'An infrastructure failure (toolchain unavailable, timeout) is staged as infrastructure, never blamed on the game.'
  ],
  limits: [{ what: 'Build timeout / heap / workers', value: '60 s / 512 MiB / 1 per instance', source: 'domain' }],
  lifecycle: BUILD_LIFECYCLE,
  sources: []
});
