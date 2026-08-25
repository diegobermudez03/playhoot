Package `composer` is a layer above the domains.

The idea is that each domain CAN NOT perform calls to external domains, it can only perform internal domain calls

Because of that, it might happen that we want a GET operation which returns data from multiple domains, in which case we wont have a single domain exposing the main method and internally calling other domains, instead, each domain will expose its own read method, and this composer pkg will build on top of it to compose the actual data and expose it.

- Only for composite read operations
- Can have read logic, for instance, given that this is cross domain read methods it can happen that:
  - We call domain X reading entity Y
  - then we want to compose with entity W from domain Z, but at this point, the entity Y is technically already stale, so whatever state we get from entity W might be from a more recent version from Y
  - in that case, entity Y can export something like version number, then the read to domain Z can request version number N from entity W, so we guarantee to get the same snapshot entities (it depends up to what each domain supports). but thats to explain that this composer can have that kindof logic
