# DISCOVERY 

Owns:
 - Reviews
 - Games discovering  (catalog, replicated from game domain)
 - Other games specific stats

## Current state

- Template catalog (2026-11-06, templates step 1): `discovery/usecases/templatecatalog` (`Featured`, `Categories`, `Search(text, category, players, page)`) is the port the game-authoring reads are built on, with the entries' shape in `discovery/templates.go` (data the database keeps: name, description, categories, players, languages, aspect ratio, devices, featured). It answers empty lists: replicating published templates from Game Management, featuring, categories and the search itself are step 2 (decided by the human 2026-11-05).
