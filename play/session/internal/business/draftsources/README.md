# draftsources

The rules of a DRAFT version's authored sources (which paths a project may hold, its limits, how a
change applies) and the starter template a new DRAFT begins with.

`template/` is a copy of the frontend repository's SDK fixture `playhoot_frontend/fixtures/sdk-games/tap-race`
(its sources only, never `dist/`) plus `notes.md`. It must keep building with the current Playhoot SDK
(`npm run game -- build <project>` in the frontend repository): when the SDK changes, copy the fixture
again.
