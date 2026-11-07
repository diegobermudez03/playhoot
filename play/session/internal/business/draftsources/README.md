# draftsources

The rules of a DRAFT version's authored sources (which paths a project may hold, its limits, how a
change applies, how find/replace edits resolve into whole files and the bounded diff answered for
them) and the starter template a new DRAFT begins with.

`template/` is a copy of the frontend repository's SDK fixture `playhoot_frontend/fixtures/sdk-games/tap-race`
(its sources and its saved `scenarios/`, never `dist/`) plus `notes.md`, and an empty `brief.json` in phase `brief` (the fixture's own brief describes Tap Race as finished; a new game starts by agreeing what it will be). It must keep building with the current Playhoot SDK
(`npm run game -- build <project>` in the frontend repository): when the SDK changes, copy the fixture
again.
