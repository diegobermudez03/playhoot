- game management `GetPlayableGameWithCurrentVersion` returns program type, that breaks the concept of that the signature of a method can only return its own types, that method is technically accesible from outside the domain, and it'd expose the program definition
  - I think that thats wrong, but then how does session get the definition? I think the definition should be part of the session domain
  - the session subdomain should be the one storing the definition, management should only care about that, management responsability (visibility, status, images, reviews, etc). Not about its execution, its compiling, its definition even
  - that would imply having a tablle that represents the game in the management domain, but also another table that represents it in the session one
  - then both program and engine subpkgs can become session only internal implementations
  - then we could even achieve to decouple completely management from session pkgs, and then they wont need to be inside the same /game directory, and then they can be independently deployable. because then the create session operation doesnt even need to fetch the game from managemnt, or it can only to validate visibility, but from then on, the next session steps will fetch the definition directly from its own DB, doesnt need to interact with management once the session exists

- Note for me when thinking about metrics (monitoring.Alert is the equivalent of panic, no tags, not queryable apart from what appears in log, metrics will be queryable)

- Manager should not have the db field, and it should not expose the GetDB method:
  - is its repo the one thatkeeps the db and the one that implements the GetDB interface, therefore is the one passed to the run in tx method

- Every service-db interaction must always go through a mockable method call (AKA repository)
  - idempotency pkg breaks that, manager is directly coupling with idempotency pkg which directly calls db
  - but this is a separate issue which solves both things at the time, zlet me explain in next message

- idempotency shouldn't be at whole domain level, it should be at workflow level, meaning that, the sessionlifecycle workflow can implement the idempotency code (DB calls, writes, reads)
  - the idempotency tables should be per workflow, not per domain
  - because we might have many workflows, each one handling idempotency differently
  - the way we want to keep track of requests depends on the request itself, meaning, the workflow
    - this is behavior driven design, not entity or domain driven, a single whole request table with fixed behavior for the whole domain breaks the behavior driven
      - but having idempotency/request tracking as workflow centric makes it behavior driven, how idempotencyt/request works depends on the needs of that specific workflow, its tables, its operations, etc
      - then, we need no separate idempotency pkg, then the idempotency calls come through repo which is mockable (interface)

`	// ResolveSessionForJoinCode resolves the most recent join_codes row for
	// this code number regardless of revocation status (never filtered to
	// revoked_at IS NULL): a code that was active a moment ago can be
	// concurrently revoked by another operation's lazy lobby-expiration
	// materialization before this call reaches the lock below, and that race
	// must still surface as the ordinary LobbyExpired outcome value once
	// locked, not as a hard "invalid code" error discovered here and never
	// re-validated under lock. A code already revoked independently of the
	// Session it names remaining LOBBY (resolution.RevokedAt set) is instead
	// rejected once locked, immediately below - see joinSessionInTx.`

    the type of comment above is the wrong type of comment, why are we commenting a method call explaining what the methiood will do? thats completely wrong, the called method itself will have a method in its signature, thats the method documentation, if reader wants to know what that call will do, then the reader will check the called method comment,

`	// Loads the Session's pinned Definition/Version UUID directly - never
	// the Game's current version - before opening the mutation
	// transaction/row lock, so lobby capacity stays governed by the exact
	// version this Session was pinned to at Create.`
another example, these are just 2 examples, but we must fix this everywhere and add standards to avoid this in the future

- join_codes.code is indexed right?
- Regarding `ResolveSessionForJoinCode`, either revoked concept shouldnt exist and just be soft deleted, or we should filter by revoked at is null
  - I think we allow to use same join code for multiple sessions, only 1 active at the time, but htis query is limiting to 1, what if there's a session with valid join code but we're getting the old expired session only always?

- LockByID: i think we should also move locking to workflow centric, not domain centric

- MaterializeIfDue: I think we have already discussed about the pattern of receiving repo, I stated that we should only receive interface params when its for a valid behavior driven no side effect purpose, like a factory pattern where we allow to receive an interface to apply different type of compute operations on some input, etc. But we shouldn't use this pattern for dependencies, repository is a dependency, dependency means when what it exposes interacts with the external world, in this case DB, meaning that if a function requires that, then the function must be a method of the struct that has access to that dependency, in short, the function should be a method of Manager
  - voice to text: "Analizando no entiendo por qué esto está así, porque el patrón es ese y es por algo mismo que yo te dije y tú no encontraste una forma mejor de hacerlo y yo lo entiendo, no te estoy culpando. Lo que veo que sucedió es que yo te di la regla de que no se podía recibir el repository como una interface en una función. Pero también te di la regla de que quería que el nivel principal del paquete Session Lifecycle solo tuviera archivos dedicados a los pasos, ¿verdad? y a los métodos que hacen parte de un paso en específico. Pero que para los métodos que son compartidos para todos los pasos no tuviéramos un archivo distinto como expiration.go, sino que lo moviéramos a la carpeta internal y ahí tuviéramos esa lógica compartida. Entonces lo que te tocó a ti fue mover esa lógica compartida de expiración a un paquete interno. Y al ser un paquete interno ya no puede ser un método del manager, lógicamente. Entonces, si tienes un método que no hace parte de manager, pues tiene que ser una función y cómo interactuar con el repository, pues tiene que interactuar por medio de recibirlo, el repository. Entonces entiendo eso, entiendo esa situación, pero creo que tenemos que hacer algo al respecto. Y veo dos opciones, te las voy a comentar y si tú tienes otra opción me puedes decir. La primera opción que veo es devolvernos, romper mi estándar que yo agregué ahí, o sea agregar tal vez una cláusula, yo qué sé, poner excepciones y que esta excepción sea mover el paquete de expiration nuevamente a la capa principal del Session Lifecycle y por ende que es un archivo expiration.go y por ende el método ahora será parte del manager y así interactuará con el repository. La segunda opción sería mantenerlo en un paquete aislado interno, pero ahora no sería una función suelta, tendría su propia estructura que sería un, digamos expirer, expirador, no sé, imagínate el nombre que sea, y eso pasaría a ser una dependencia del manager de Session Lifecycle. Entonces el manager de Session Lifecycle tiene una dependencia que es el expirador. El expirador no es nada más que esa estructura que se usa por medio de una interface que tiene una implementación interna. Internamente, entonces en el paquete de expiration, ese paquete, esa estructura recibiría como dependencia el repository o la base de datos para que él mismo cree su repository y acá sería una importación, una dependencia del mismo nivel, porque el paquete expiration y el paquete repository están al mismo nivel de profundidad, lo cual es permitido. Entonces se me ocurren esas dos opciones y pues te las comento y podemos hablar cuál es mejor."

- lo mismo de arriba para el capture pkg
- i might be missing context regarding how game UI works, but are those presentation outputs enough? I thought we had position, cards, and complex dynamic UI rendering, also with images
