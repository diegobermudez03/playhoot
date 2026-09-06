# Data Integrity Failure Standard

Status: CANONICAL ENGINEERING STANDARD

This standard applies to unexpected persisted/runtime state and similar internal inconsistencies.

## Rules

1. Panic only when the inconsistency is severe enough that continuing the process is unsafe and callers must also abort.
2. An unexpected or inconsistent value does not automatically justify panic.
3. If the system can safely stop the current logical operation and return a normal logical/domain error:
   - do not panic;
   - emit an alert using the monitoring abstraction;
   - return the appropriate error.
4. Monitoring/alerting is used so unexpected internal inconsistency becomes operationally visible even when the process can safely continue.

This standard does not define alert thresholds, alert provider, metrics strategy, dashboards, tracing, or general observability architecture.

The abstraction and semantic intent are canonical, not the current backend implementation details.
