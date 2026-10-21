# Seed data files

Test content embedded by the seed migrations (never used in production).

- `space_drift_frontend.html`: Space Drift's frontend package, built by the frontend repository
  (`playhoot_frontend`, `fixtures/games/space-drift`, `npm run fixtures`, output
  `fixtures/games/space-drift/dist/index.html`) with the Playhoot Game SDK. One self-contained
  HTML file. To change it, rebuild it there, copy it here and add a NEW seed migration with a new
  Space Drift version: a GameVersion's frontend package never changes once a Session may have
  pinned it.
- `parques_frontend.html`: Parqués' frontend package, built the same way from
  `fixtures/games/parques` (output `fixtures/games/parques/dist/index.html`). The same rule applies:
  to change it, add a NEW seed migration with a new Parqués version.
