# Metrics Standard

Status: CANONICAL ENGINEERING STANDARD

This standard owns how operational metrics are declared, who is responsible for each kind of metric, and what callers may and may not do when recording them. It does not define the metrics provider, dashboards, alert thresholds, or tracing; the alerting abstraction is described in `data-integrity.md`.

## Entry Point And Contract

Metrics are recorded through the `monitoring/metrics` package. Its exposed interface is not infrastructure-aware (no provider, registry, or exporter concept leaks to callers); the package's own implementation may be specific to the deployment platform.

Supported kinds: counters (only go up), gauges (go up and down), and histograms (distribution into declared buckets). A new kind is added to the package, not hand-rolled by a caller.

## The Owner Of The Logic Owns The Metric

A metric is declared and recorded by the domain/package that owns the logic whose outcome it describes. The `metrics` package provides only the generic mechanism and declares no domain metric itself.

- **The domain owns the source of truth for its business outcomes.** Only the owning domain can define how an operation is evaluated. For example, how many Sessions were created is counted by the Session domain, where the creation actually happens, never by an endpoint that calls it.
- **An entry point is not the source of truth.** An endpoint (or any other way into an operation) only sees its own transport. A "sessions created" counter recorded by one endpoint would be wrong as soon as there is a second way to create a Session. Entry points therefore do not record business-outcome metrics.
- **Entry points measure their own transport, uniformly.** The API edge measures every REST route the same way, centrally and automatically in `restObservability`, so no handler declares anything for it:
  - requests finished, by matched route and HTTP status code (the HTTP status is the success/error signal at this level);
  - request duration, by matched route (to see which endpoints are slowest and the typical duration per request type).

  Handlers do not record request counts, outcomes, or durations themselves.
- **A component may measure its own behavior.** A middleware or other edge component may declare metrics about what it itself does, as the component that owns that behavior. For example, the authorization middleware counts its checks by outcome (allowed, unauthenticated, forbidden, missing token, error) and the time Identity took to answer. These describe the middleware, not Identity's domain; they do not replace metrics Identity may declare about its own logic.

## Labels Have Closed Value Sets

A label's values are declared up front. A caller can never record a label value that was not declared, so a metric's cardinality is bounded by construction.

- A `Label` is created once with its name and its complete set of allowed values; recording uses `LabelValue`s obtained from `Label.Value`, which are resolved where the metric is declared, not per call.
- No free-form string (a user id, a session id, an error message, a raw URL) is ever a label value.
- A metric's total label combinations are capped by the package; a declaration that would exceed the cap fails.
- Where the closed set is only known once the system is assembled (for example, the set of routes), the metric is declared at that point, by the component that knows the set.
- Declaring the same label or metric again with exactly the same definition returns the existing one; the same name with a different definition is a programming error.

## Misuse Fails Loudly

Invalid names, conflicting redeclarations, an undeclared or missing label value, a decreasing counter, or a non-finite value are programming errors and panic. Declarations are resolved at startup so they surface there rather than on a live request.

## Every Call Carries Context And Is Logged

Every recording call takes a `context.Context` and also writes the measurement into that context's request log, the way `monitoring.Alert` does, so a point seen in a metric can be matched with the logs of the requests that produced it. A context without a started request log records the metric but writes nothing to a log.

Avoid recording from a hot loop with a context that carries a request log, since the request log grows with every call.

## Naming

Metric and label names are lower snake_case. Counters end in `_total`; duration histograms are in seconds and end in `_seconds`. Every metric has help text.

## Enforcement

Code review and this standard; the `metrics` package enforces the closed-label and naming rules at declaration time. Flag in particular:

- a handler or other entry point recording a business-outcome metric or its own duration;
- a metric declared outside the domain that owns the outcome it counts;
- an attempt to build a label value from dynamic input;
- a hand-rolled counter or timer instead of the `metrics` package.
