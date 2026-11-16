// Phase 1 flow catalog (T-002). Every cross-domain operation of the base and the patches,
// in state planned: name, coordinator, domains it touches and sources. Each milestone's
// design task replaces "touches" with real steps, persistence and idempotency.
// create-session is not here: it is already implemented and lives in orchestrator.js.

// ---------- Identity and access ----------
// H3 steps designed by T-300. Switching modes is identity-only (identity.set-mode, D-034), so the
// old switch-mode flow entry is gone: entitlements never ride the token (P15§5) and my-data is
// where the frontend learns what a mode has behind it.
PH.flow({
  id: 'email-code-sign-in', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Email code sign-in',
  summary: 'Signs in or registers with a one-time code sent by email (P03§8). The code travels as a notify task (type access-code, necessary email) through the existing deliver-notification and send-notification-email flows; identity resolves the attempt to its address, so notifications never stores one.',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.email-login', when: 'the person asks for a code (StartEmailLogin)', onFailure: 'Rate refusals and unknown addresses answer the same neutral acknowledgement: nothing reveals whether an address has an account.' },
    { n: 2, domain: 'identity', operation: 'identity.email-login', when: 'the person submits the code (CompleteEmailLogin)', onFailure: '5 wrong codes exhaust the attempt; an expired, superseded or exhausted attempt asks for a new code.' }
  ],
  persistence: 'queue',
  idempotency: 'One live attempt per address (a new code supersedes it); completion consumes the attempt once; the notify task is keyed by the attempt.',
  compensation: 'A lost or undelivered email is recovered by asking for a new code (P31§3); nothing else to undo.',
  sources: ['P03§8', 'P03§10', 'P31§3-4']
});
PH.flow({
  id: 'delete-account', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Delete account',
  summary: 'Withdraws access immediately, then a queued cleanup clears what identifies the person across the domains; shared history survives with a neutral, distinguishable reference (P02§10). H5 (replay content), H6 (billing) and H8 (classroom) append their steps when those domains hold user data.',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.delete-user', when: 'always, synchronously: access ends before the call returns', onFailure: 'Propagate; nothing was half-deleted (one transaction).' },
    { n: 2, domain: 'game', operation: 'game.delete-user-games', when: 'the account-cleanup task runs', onFailure: 'The queue retries; a dead cleanup task is an admin alert (V-12).' },
    { n: 3, domain: 'play.session', operation: 'play.session.anonymize-user', when: 'always, after the games', onFailure: 'The queue retries; a dead cleanup task is an admin alert (V-12).' },
    { n: 4, domain: 'notifications', operation: 'notifications.delete-user-entries', when: 'always', onFailure: 'The queue retries; a dead cleanup task is an admin alert (V-12).' }
  ],
  persistence: 'queue',
  idempotency: 'The cleanup task is keyed by the user uuid and every step is idempotent: running it again deletes nothing new.',
  compensation: 'None: deletion is not undone. Compliance evidence stays restricted (P02§11, resolved by H11); analytics events stay but their uuid no longer resolves to anyone (D-036).',
  sources: ['P02§10-11', 'P04§6']
});
PH.flow({
  id: 'expire-guests', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Expire inactive guests',
  summary: 'Daily sweep: a guest 90 days without session use goes through the same deletion and cleanup as a requested one (the delete-account steps); the term is disclosed where the guest is created.',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.expire-guests', when: 'the daily periodic task on the identity queue is due', onFailure: 'The queue retries; a partial sweep continues on the next run.' }
  ],
  persistence: 'queue',
  idempotency: 'A guest already deleted is skipped; batches of 500 until none are left (V-11). Each expired guest leaves its own account-cleanup task.',
  compensation: 'None; shared history keeps working through the same neutral reference as deletion.',
  sources: ['P02§9-10']
});
PH.flow({
  id: 'my-data', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'My data',
  summary: 'One read that tells the frontend who the caller is: identity and mode, interface language, linked methods, the current declaration, and the capacities the plan gives (B: identity, P03§9). The shell keeps reading identity from the credential (D-028); this serves the profile and mode-dependent screens.',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.read-profile', when: 'always', onFailure: 'Propagate: without an identity there is no answer.' },
    { n: 2, domain: 'compliance', operation: 'compliance.read-declaration', when: 'the caller is an account', onFailure: 'Answer without the declaration and say so; the profile shows it as unknown.' },
    { n: 3, domain: 'plan', operation: 'plan.get-user-limits', when: 'always', onFailure: 'Answer without capacities and say so.' }
  ],
  persistence: 'none',
  idempotency: 'Read only.',
  compensation: 'None.',
  sources: ['B: identity', 'P03§9', 'P15§5']
});

// ---------- Games, publication and Discovery ----------
PH.flow({
  id: 'create-game', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Create game',
  summary: 'Creates the logical game (optionally from a template) and its first draft version, idempotently. H3 adds the market-and-age gate (step 1, planned, D-038); capacity checks arrive with H4 (see GAPS).',
  steps: [
    { n: 1, domain: 'compliance', operation: 'compliance.check-market-access', when: 'always once H3 wires the gate (planned): function create-games, place from the declared residence or the approximate country', onFailure: 'needs-declaration sends the person to the declaration screen and back (D-038); a blocked market refuses with a service-restriction notice.' },
    { n: 2, domain: 'game', operation: 'game.find-template', when: 'a template game uuid was given', onFailure: 'Continue without a template if not found.' },
    { n: 3, domain: 'game', operation: 'game.game-lifecycle', when: 'always', onFailure: 'Propagate the conflict (same key, different name or template).' },
    { n: 4, domain: 'play.versions', operation: 'play.versions.create-draft-version', when: 'the game was created', onFailure: 'Retry with the same key completes the missing step.' }
  ],
  persistence: 'none',
  idempotency: 'The Idempotency-Key is passed through; a retry with the same key finishes whatever step did not complete.',
  compensation: 'None: the last step failing leaves an unpublished game with no draft, repaired by the retry.',
  sources: ['B: create game', 'P19§1', 'P27', 'P35§5']
});
PH.flow({
  id: 'request-publication', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Request publication',
  summary: 'Submits a specific version + card + assets combination for review; Discovery keeps the previously approved one meanwhile.',
  touches: ['game', 'play.versions', 'moderation'],
  sources: ['P04§4']
});
PH.flow({
  id: 'review-publication', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Review publication',
  summary: 'The admin approves or denies the submitted combination; the outcome lands in the game\'s public state and notifies the creator.',
  touches: ['moderation', 'game', 'notifications'],
  sources: ['P04§4', 'P31§4']
});
PH.flow({
  id: 'edit-public-card', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Edit card of a public game',
  summary: 'Name, description and cover image are edited apart from versions; when the game is public the change goes through review.',
  touches: ['game', 'moderation'],
  sources: ['P04§2', 'P04§4']
});
PH.flow({
  id: 'withdraw-game', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Unpublish or delete game',
  summary: 'Ordinary withdrawal or deletion; existing Sessions and other people\'s replays are untouched.',
  touches: ['game', 'play.versions'],
  sources: ['P04§6']
});
PH.flow({
  id: 'switch-current-version', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Change offered version',
  summary: 'Keeps the current version playable while the next is prepared, makes the game unavailable, or returns to a previous available and authorized version.',
  touches: ['game', 'play.versions'],
  sources: ['P04§5']
});
PH.flow({
  id: 'enable-template', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Enable game as template',
  summary: 'Explicit acceptance of the lasting reuse permission; assets not authorized for reuse are excluded or replaced first.',
  touches: ['game', 'play.versions', 'library'],
  sources: ['P05']
});
PH.flow({
  id: 'copy-template', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Copy template',
  summary: 'Creates an independent game from a specific version keeping provenance and credits; consumes one creation; private context never carries over.',
  touches: ['plan', 'game', 'play.versions', 'library'],
  sources: ['P05', 'P12§14']
});
PH.flow({
  id: 'browse-discovery', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'Browse Discovery',
  summary: 'Popular, categories, recent, text search and filters over approved content, with real statistics or none.',
  touches: ['game', 'play.session'],
  sources: ['B: search', 'P14§2']
});
PH.flow({
  id: 'view-game-card', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'View game card',
  summary: 'The public card: languages, players, devices, academic data, template mark, screenshots of the offered version and statistics.',
  touches: ['game', 'play.versions', 'play.session'],
  sources: ['P14§2']
});

// ---------- Sessions, history and replays ----------
PH.flow({
  id: 'join-session', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Join session',
  summary: 'Admits a participant: the H3 market gate at entry, then the existing eligibility checks and the seat; a guest is asked only for a display name (C-01). The H5 design extends admission (host modes, close/reopen, kick, 60 s) and H8 adds class or group membership through classroom.',
  steps: [
    { n: 1, domain: 'compliance', operation: 'compliance.check-market-access', when: 'always at admission (H3): function interactive-use, place from the declared residence or, for a guest, the approximate country only (C-01)', onFailure: 'A blocked market refuses with a service-restriction notice; unknown location admits (the local locator always answers unknown).' },
    { n: 2, domain: 'play.session', operation: 'play.session.check-join', when: 'the market allows interactive use', onFailure: 'Refusals keep their existing outcomes (full, started, unknown code).' }
  ],
  persistence: 'none',
  idempotency: 'Read-side checks; the seat itself stays idempotent per user inside play.session.',
  compensation: 'None.',
  sources: ['B: session', 'P06§2', 'P35§5', 'C-01']
});
PH.flow({
  id: 'start-session', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Start session',
  summary: 'The host starts the game once restrictions are met; the use reserved at the lobby is consumed.',
  touches: ['play.session', 'plan'],
  sources: ['P06§2', 'P11§10', 'P12§15']
});
PH.flow({
  id: 'close-session', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Close session',
  summary: 'Any closing reason; a cancellation or expiry before starting releases the reserved use, and a confirmed Playhoot failure restores it.',
  touches: ['play.session', 'plan'],
  sources: ['P06§7', 'P06§10', 'P12§15']
});
PH.flow({
  id: 'repeat-session', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Repeat session',
  summary: 'A new lobby with the same game, version, configuration and class, without overwriting the closed one.',
  touches: ['game', 'classroom', 'plan', 'play.session'],
  sources: ['P06§12']
});
PH.flow({
  id: 'session-history', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'Session history',
  summary: 'Played and hosted Sessions with filters, per session and per game; detail shows the game or "no longer available".',
  touches: ['play.session', 'game'],
  sources: ['B: history', 'P06§11']
});
PH.flow({
  id: 'watch-replay', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'Watch replay',
  summary: 'Plays a recording back with the perspectives the viewer had in the Session; missing segments are declared.',
  touches: ['play.replay', 'play.session', 'game'],
  sources: ['B: history', 'P02§10', 'P06§1']
});

// ---------- Creation ----------
PH.flow({
  id: 'start-creation', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Start creation',
  summary: 'Same underlying mechanism as create-game: creates the game and its first draft version. Capacity checks, the conversational brief and route selection (own AI vs. integrated chat) are not implemented yet (see GAPS); today the creator reaches the draft directly.',
  steps: [
    { n: 1, domain: 'game', operation: 'game.game-lifecycle', when: 'always', onFailure: 'Propagate the conflict (same key, different name or template).' },
    { n: 2, domain: 'play.versions', operation: 'play.versions.create-draft-version', when: 'the game was created', onFailure: 'Retry with the same key completes the missing step.' }
  ],
  persistence: 'none',
  idempotency: 'The Idempotency-Key is passed through; a retry with the same key finishes whatever step did not complete.',
  compensation: 'None: step 2 failing leaves an unpublished game with no draft, repaired by the retry.',
  sources: ['B: create game', 'P09§1', 'P12§14']
});
PH.flow({
  id: 'run-ai-job', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Run integrated AI job',
  summary: 'Admits the cost, reserves credits up to the authorized cap, runs the job (models, assets, web research), applies changes to the draft, then settles: consumed work is charged, failures without usable result are not.',
  touches: ['creation', 'plan', 'budget', 'library', 'play.versions'],
  sources: ['P09', 'P12§8-10', 'P24', 'P32', 'P37']
});
PH.flow({
  id: 'authorize-external-ai', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Authorize external AI',
  summary: 'Creates, renews or revokes the read-only link and the separate modification authorization of a draft; 7-day expiry, renewal invalidates the previous one.',
  touches: ['creation', 'identity'],
  sources: ['P10§1-3']
});
PH.flow({
  id: 'external-ai-edit', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'External AI edits draft',
  summary: 'An AI authenticated by a game link (identity.resolve-resource-link) or by its own MCP OAuth connection acts as the owner and applies changes, uploads assets, or simulates, exactly like a human editor. Cost admission and billable metering of the AI\'s work are not implemented yet (see GAPS).',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.resolve-resource-link', when: 'the AI calls through its /ai/v1/{token} link', onFailure: 'Stop with ai_link_invalid.' },
    { n: 2, domain: 'game', operation: 'game.find-editable-version', when: 'always', onFailure: 'Stop with not_found or not_editable.' },
    { n: 3, domain: 'play.versions', operation: 'play.versions.apply-changes', when: 'the AI sends file edits, a template, or a restore', onFailure: 'Return its outcome: conflict, invalid or not_editable.' }
  ],
  persistence: 'none',
  idempotency: 'The AI is instructed to send a fresh idempotency key per change; play.versions records each attempt once per version and key.',
  compensation: 'None needed: each step is transactional within its own domain.',
  sources: ['P10§4-5', 'P32']
});
PH.flow({
  id: 'finalize-version', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Finalize version',
  summary: 'The creator explicitly declares the version finished; ready checks pass and the version freezes, assets included.',
  touches: ['creation', 'play.versions', 'game'],
  sources: ['P04§1', 'P09§2-3']
});
PH.flow({
  id: 'report-creation-error', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Report creation error',
  summary: 'The user or the AI (directly or via a link with text to paste) reports a creation error linked to the game.',
  touches: ['support', 'creation'],
  sources: ['B: create game', 'P10§4']
});
PH.flow({
  id: 'creation-workspace', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'Creation workspace',
  summary: 'Everything a creator (or an external AI through its link) sees about a draft: brief, progress, validation results, authorizations.',
  touches: ['creation', 'game', 'play.versions'],
  sources: ['P09§6', 'P10§4']
});

// ---------- Classroom ----------
PH.flow({
  id: 'create-class', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Create class',
  summary: 'A teacher creates a class within their active-classes capacity; reactivating an archived class asks for capacity again.',
  touches: ['plan', 'classroom'],
  sources: ['B: classroom', 'P27§1']
});
PH.flow({
  id: 'join-class', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Student joins class',
  summary: 'Request via class link or code plus teacher approval (an invitation may include it); authorization coverage is checked, the student identity and its private credential are issued.',
  touches: ['classroom', 'compliance', 'identity'],
  sources: ['P03§2-5', 'P20§3']
});
PH.flow({
  id: 'reissue-student-credential', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Reissue student credential',
  summary: 'The teacher reissues a student\'s credential; the previous one and its sessions are invalidated.',
  touches: ['classroom', 'identity'],
  sources: ['P03§6']
});
PH.flow({
  id: 'register-authorizations', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Register existing authorizations',
  summary: 'The teacher records coverage per student (who keeps the evidence, origin, scope, validity), also in batch; missing coverage blocks the student, not the class.',
  touches: ['compliance', 'classroom'],
  sources: ['P20']
});
PH.flow({
  id: 'guardian-authorization', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Guardian authorization',
  summary: 'The legal guardian confirms their email by code, sees organizer, data, purposes and retention, accepts expressly and gets a receipt; lookup and withdrawal stay available.',
  touches: ['compliance', 'notifications'],
  sources: ['P21§1-3']
});
PH.flow({
  id: 'withdraw-authorization', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Withdraw authorization',
  summary: 'A guardian withdraws authorization; the student is blocked and the teacher cannot override it.',
  touches: ['compliance', 'classroom', 'notifications'],
  sources: ['P21§3']
});
PH.flow({
  id: 'family-group-membership', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Family group membership',
  summary: 'An adult creates a named private group and invites by link; each minor enters with a per-group identity authorized by their own guardian.',
  touches: ['classroom', 'compliance', 'identity'],
  sources: ['P21§4', 'P35§4', 'C-02']
});
PH.flow({
  id: 'upload-material', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Upload material',
  summary: 'Adds a material to the teacher\'s space within storage capacity and upload rules; territorial control applies at prior data upload.',
  touches: ['compliance', 'plan', 'budget', 'classroom'],
  sources: ['P07§4-5', 'P27§5', 'P35§5']
});
PH.flow({
  id: 'create-class-activity', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Create class activity',
  summary: 'Prepares the use of a game in a class, pinning version, configuration and grading; it can be created on its own when the first Session starts.',
  touches: ['classroom', 'game', 'play.versions'],
  sources: ['P07§1', 'P08§1']
});
PH.flow({
  id: 'create-class-session', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Create class session',
  summary: 'Creates a Session for a class activity; only linked students join and link or code does not bypass that (the H8 design decides how this extends create-session).',
  touches: ['classroom', 'plan', 'game', 'play.session'],
  sources: ['B: classroom', 'P07§1']
});
PH.flow({
  id: 'collect-grades', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Collect grades',
  summary: 'Fetches the original results of a class\'s Sessions and hands them to classroom, which relates them to students and manages attempts and review.',
  touches: ['classroom', 'play.session'],
  sources: ['B: classroom', 'P08§2-5']
});
PH.flow({
  id: 'close-class', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Close class',
  summary: 'Stops enrollments and class sessions; viewing and export remain during retention.',
  touches: ['classroom', 'play.session'],
  sources: ['P07§6', 'P02§11']
});
PH.flow({
  id: 'class-retention', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Class retention sweep',
  summary: 'Notifies and archives open classes unused for 12 months; closed classes expire 24 months after closing, extendable if valid.',
  touches: ['classroom', 'notifications'],
  sources: ['P02§9', 'P02§11']
});

// ---------- Plans, purchases and credits ----------
PH.flow({
  id: 'purchase', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Purchase',
  summary: 'Subscription or one-off purchase: function availability, benefit usability and budget coverage are checked, then checkout opens with full disclosure before paying.',
  touches: ['compliance', 'plan', 'budget', 'billing'],
  sources: ['P11§1-2', 'P23', 'P27§4', 'P30']
});
PH.flow({
  id: 'confirm-payment', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Confirm payment',
  summary: 'Reliable provider confirmation activates the purchased benefits idempotently; a confirmed payment with failed activation recovers on its own and is never shown as rejected.',
  touches: ['billing', 'plan', 'notifications'],
  sources: ['P11§6', 'P15§6']
});
PH.flow({
  id: 'renew-subscription', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Renew subscription',
  summary: 'Charges the renewal and refreshes period pools; a failed renewal opens 7 days of recovery without new pools.',
  touches: ['billing', 'plan', 'budget', 'notifications'],
  sources: ['P11§8', 'P31§4']
});
PH.flow({
  id: 'change-plan', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Change plan',
  summary: 'Upgrade is immediate after paying the proration and keeps the date; downgrades and mixed changes apply at renewal; consumption is not reset.',
  touches: ['billing', 'plan'],
  sources: ['P11§4']
});
PH.flow({
  id: 'cancel-subscription', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Cancel subscription',
  summary: 'Simple cancellation, benefits until the end of the period, undoable before it.',
  touches: ['plan', 'billing', 'notifications'],
  sources: ['P11§5']
});
PH.flow({
  id: 'refund', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Refund',
  summary: 'Request and resolution from "My purchases": first monthly fee within 14 days without use of paid benefits, duplicates, or the unconsumed part of purchases.',
  touches: ['billing', 'plan', 'notifications'],
  sources: ['P11§13-15']
});
PH.flow({
  id: 'handle-dispute', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Handle dispute',
  summary: 'A payment dispute suspends that payment\'s benefits and keeps disputed uses reserved; it does not ban or delete.',
  touches: ['billing', 'plan', 'notifications'],
  sources: ['P11§16']
});
PH.flow({
  id: 'activate-teacher-trial', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Activate teacher trial',
  summary: '7 days, once per account, no card, no automatic subscription; requires reserved budget; its pools are independent and expire at the end.',
  touches: ['compliance', 'budget', 'plan', 'notifications'],
  sources: ['P11§7', 'P28§1-2']
});
PH.flow({
  id: 'my-purchases', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'My purchases',
  summary: 'Purchases, charges, refund requests and their tracking.',
  touches: ['billing', 'plan'],
  sources: ['P11§14']
});
PH.flow({
  id: 'plan-and-usage', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'Plan and usage',
  summary: 'Entitlements and capacities next to actual usage (each owning domain counts its own) and credit movements per request.',
  touches: ['plan', 'game', 'play.session', 'creation'],
  sources: ['B: plan', 'P12§11']
});

// ---------- Budget control (steps designed by T-100) ----------
PH.flow({
  id: 'spending-restriction', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Spending restriction change',
  summary: 'The ladder moves (restriction, selective pause, automatic emergency shutdown) and the reason is communicated as a service restriction, never as lack of balance.',
  steps: [
    { n: 1, domain: 'budget', operation: 'budget.evaluate-ladder', when: 'after spend ingestion and on the periodic queue task', onFailure: 'The queue retries; missing metrics themselves restrict admission.' },
    { n: 2, domain: 'notifications', operation: 'notifications.accept-entry', when: 'the level changed (notify task left by step 1, addressed to the admin; it waits in the queue until Notifications registers delivery)', onFailure: 'The queue retries; the level change itself is already in force.' }
  ],
  persistence: 'queue',
  idempotency: 'Each ladder move appends exactly one change row; the notify task carries that row’s key, so retries never duplicate the notification.',
  compensation: 'None: de-escalation below shutdown is automatic when pressure clears; shutdown only leaves through reopen-spending.',
  sources: ['P32§3-6']
});
PH.flow({
  id: 'adjust-cost-controls', status: 'verified', coordinator: 'orchestrator', effect: 'write',
  name: 'Adjust cost controls',
  summary: 'The admin changes caps or rates with reinforced authentication; every change keeps actor, reason and history.',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.verify-totp', when: 'always (step-up proof for this action)', onFailure: 'Stop: no control changes without a fresh second factor.' },
    { n: 2, domain: 'budget', operation: 'budget.admin-set-controls', when: 'the proof is valid', onFailure: 'Propagate; nothing was half-written (append-only rows in one transaction).' }
  ],
  persistence: 'none',
  idempotency: 'The step-up proof is single-use; replaying the request without a new code is refused.',
  compensation: 'None needed.',
  sources: ['P32§2', 'P32§7']
});
PH.flow({
  id: 'emergency-cutoff', status: 'implemented', coordinator: 'orchestrator', effect: 'write',
  name: 'Admin emergency cutoff',
  summary: 'The admin imposes an immediate shutdown with a reason, behind the second factor.',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.verify-totp', when: 'always', onFailure: 'Stop.' },
    { n: 2, domain: 'budget', operation: 'budget.admin-cutoff', when: 'the proof is valid', onFailure: 'Propagate; retrying while shut down is idempotent.' }
  ],
  persistence: 'none',
  idempotency: 'Cutting off an already shut down platform appends nothing.',
  compensation: 'None: recovery is reopen-spending.',
  sources: ['P32§7']
});
PH.flow({
  id: 'reopen-spending', status: 'verified', coordinator: 'orchestrator', effect: 'write',
  name: 'Reopen after shutdown',
  summary: 'Only an explicit manual reopening with second factor lifts the shutdown, gradually; restart, deploy or month change never do.',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.verify-totp', when: 'always (step-up proof for this action)', onFailure: 'Stop: the shutdown stays.' },
    { n: 2, domain: 'budget', operation: 'budget.reopen', when: 'the proof is valid', onFailure: 'Propagate; the shutdown stays until a successful reopening.' }
  ],
  persistence: 'none',
  idempotency: 'Each gradual step appends its own change row; repeating a step is refused as already open.',
  compensation: 'None: the ladder can always be cut again.',
  sources: ['P32§4', 'P32§7']
});

// ---------- Cross-domain queue delivery (steps designed by T-100) ----------
// A domain leaves a task in its own queue inside its own transaction; the orchestrator-registered
// processor delivers it to the owning domain (D-010). Domains never call each other.
PH.flow({
  id: 'deliver-notification', status: 'verified', coordinator: 'orchestrator', effect: 'write',
  name: 'Deliver notification',
  summary: 'Processes a notify task from any domain’s queue into one logical notification entry.',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.read-contact', when: 'a notify task is due (recipient admin resolves to the active administrator)', onFailure: 'No administrator yet: the task retries. Unknown recipient: the task turns dead at once.' },
    { n: 2, domain: 'compliance', operation: 'compliance.get-acceptance-text', when: 'the type is marketing', onFailure: 'The queue retries.' },
    { n: 3, domain: 'notifications', operation: 'notifications.accept-entry', when: 'always, with whether the recipient has an address, is a student and consented to marketing', onFailure: 'The queue retries with backoff; an invalid notice turns the task dead at once (admin alert).' }
  ],
  persistence: 'queue',
  idempotency: 'The task carries the source domain’s idempotency key; acceptance replays the existing entry.',
  compensation: 'None.',
  sources: ['B: notifications', 'P29§1']
});
PH.flow({
  id: 'send-notification-email', status: 'verified', coordinator: 'orchestrator', effect: 'write',
  name: 'Send notification email',
  summary: 'Processes an email task: reads the current address, admits the cost, sends through the selected provider, releases the reservation.',
  steps: [
    { n: 1, domain: 'identity', operation: 'identity.read-contact', when: 'the email task is due (address and language read now, never stored by notifications)', onFailure: 'The queue retries.' },
    { n: 2, domain: 'budget', operation: 'budget.admit-work', when: 'always (category email, one email at the SES rate; optional email is discretionary, necessary email is not)', onFailure: 'The task waits and retries: a budget refusal is a pause, not a loss; the in-app entry already exists.' },
    { n: 3, domain: 'notifications', operation: 'notifications.send-email', when: 'admitted', onFailure: 'Resend only out of quota or a clear provider failure: the task retries later. Accepted, refused or uncertain: decided for good.' },
    { n: 4, domain: 'budget', operation: 'budget.settle-work', when: 'always after step 3, with zero quantity: it only releases the reservation', onFailure: 'Logged; the reservation expires on its own.' }
  ],
  persistence: 'queue',
  idempotency: 'One email task per entry; the delivery records are checked under a lock before any provider call, so a retried task never double-sends. The measured spend is a spend-event task written with the sent record (flow ingest-spend-events), so it is never lost or counted twice.',
  compensation: 'An unreleased reservation expires on its own; the spend that happened stays recorded.',
  sources: ['P31§1-3', 'P32§1-2']
});
PH.flow({
  id: 'ingest-spend-events', status: 'verified', coordinator: 'orchestrator', effect: 'write',
  name: 'Ingest spend events',
  summary: 'Processes spend-event tasks from the domains into the budget’s spend record.',
  steps: [
    { n: 1, domain: 'budget', operation: 'budget.record-spend', when: 'a spend-event task is due', onFailure: 'The queue retries; a dead spend task is an admin alert, not a silent loss.' }
  ],
  persistence: 'queue',
  idempotency: 'By (source domain, idempotency key) inside record-spend.',
  compensation: 'None.',
  sources: ['P32§1-2', 'P12§4']
});
PH.flow({
  id: 'ingest-analytics-events', status: 'verified', coordinator: 'orchestrator', effect: 'write',
  name: 'Ingest analytics events',
  summary: 'Processes analytics-event tasks from the domains into the analytics event store.',
  steps: [
    { n: 1, domain: 'analytics', operation: 'analytics.ingest-events', when: 'an analytics-event task is due', onFailure: 'A storage failure retries; an unreadable payload or an invalid event turns the task dead for admin after the valid events were stored. Analytics never blocks the operation that emitted the event: the task is written in the same transaction as the emitting write and delivered asynchronously.' }
  ],
  persistence: 'queue',
  idempotency: 'By (source domain, idempotency key) per event inside ingest-events.',
  compensation: 'None.',
  sources: ['P34§3', 'B: principles']
});

// ---------- Support and moderation ----------
PH.flow({
  id: 'submit-report', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Submit report',
  summary: 'A typed report from any entry point (or the public no-account form); automatic classification runs within its cost admission.',
  touches: ['support', 'moderation', 'budget'],
  sources: ['P13§4', 'P24§1', 'P33§3']
});
PH.flow({
  id: 'apply-measure', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Apply moderation measure',
  summary: 'A graduated measure is recorded with reason, scope and duration, executed in the owning domain, and notified; a suspension that prevents using the subscription stops renewals.',
  touches: ['moderation', 'game', 'play.versions', 'play.session', 'identity', 'billing', 'notifications'],
  sources: ['P13§5-6']
});
PH.flow({
  id: 'appeal-measure', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Appeal measure',
  summary: 'An appeal case over a decision; moderation decides it, support carries the conversation.',
  touches: ['support', 'moderation'],
  sources: ['P13§6', 'P33§5']
});
PH.flow({
  id: 'my-reports', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'My reports and cases',
  summary: 'The user\'s reports and cases with news highlighted through unread notifications.',
  touches: ['support', 'notifications'],
  sources: ['P13§6', 'P14§4']
});
PH.flow({
  id: 'review-queue', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'Publication review queue',
  summary: 'The admin\'s pending reviews, each with the submitted version, card and assets.',
  touches: ['moderation', 'game', 'play.versions'],
  sources: ['P04§4']
});

// ---------- Notifications ----------
PH.flow({
  id: 'notification-list', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'Notification list',
  summary: 'Unread and historical notifications joined with the entities they point at and their destination URLs.',
  touches: ['notifications', 'game', 'play.session', 'classroom', 'billing', 'support'],
  sources: ['P14§4-5']
});

// ---------- Admin ----------
PH.flow({
  id: 'admin-user-detail', status: 'planned', coordinator: 'composer', effect: 'read',
  name: 'Admin user detail',
  summary: 'A user of any type with history, activity and relationships across the platform.',
  touches: ['identity', 'plan', 'billing', 'game', 'play.session', 'classroom', 'support', 'moderation'],
  sources: ['B: admin', 'P16§3']
});
PH.flow({
  id: 'admin-dlqs', status: 'implemented', coordinator: 'composer', effect: 'read',
  name: 'Admin dead letter queues',
  summary: 'Every domain\'s dead letter queue in one view, through the txqueue registry (infrastructure tables, D-013).',
  steps: [
    { n: 1, package: 'txqueue', when: 'always: list dead tasks across every registered domain queue', onFailure: 'A queue that cannot be read is reported as unavailable; the others still show.' }
  ],
  persistence: 'none',
  idempotency: 'Read only.',
  compensation: 'None.',
  sources: ['B: principles']
});
PH.flow({
  id: 'manage-dead-task', status: 'verified', coordinator: 'orchestrator', effect: 'write',
  name: 'Retry or cancel a dead task',
  summary: 'The admin retries (back to pending) or cancels a dead task on its owning domain queue, through the txqueue registry.',
  steps: [
    { n: 1, package: 'txqueue', when: 'always: locate the task on its domain queue and apply retry or cancel', onFailure: 'Propagate; the task stays dead.' }
  ],
  persistence: 'none',
  idempotency: 'Retrying a task that is no longer dead, or cancelling twice, changes nothing.',
  compensation: 'None.',
  sources: ['B: principles']
});

// ---------- Privacy ----------
PH.flow({
  id: 'privacy-request', status: 'planned', coordinator: 'orchestrator', effect: 'write',
  name: 'Privacy request',
  summary: 'Access, correction, deletion or withdrawal through the single entry point, even without an account, with tracking; execution reaches the owning domains.',
  touches: ['compliance', 'identity', 'classroom', 'play.replay', 'notifications'],
  sources: ['P22§5', 'P02§11']
});
