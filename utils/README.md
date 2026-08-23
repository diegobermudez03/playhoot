`utils` pkg exports shared non business logic bnehavior.

Exported can be:

- Utils to handle DB transactions
- Utils methods to handle parallel tasks
- Utils to parse types
- etc

Its just helper functions, no business logic here, no data persisting, no transport layer, this pkg should be able to be moved into a separate repository, and only needed thing to do would be to update the import path, everything else in the codebase should still work the same (what I mean is that it has no actual persisting or transport operations, because in those cases moving the pkg away wouuld affect everything as its actually running something)
