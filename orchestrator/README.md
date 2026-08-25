package `orchestrator` handles the system's orchestration and coreography

Given the principles of how the system is being built, we consider each domain as a completely separarted service logically, so any call between domains is an external service call which wont have DB transaction, and therefore it has to handle all that comes with that (eventual consistency, compensation, etc).

But we dont want to have this cross service communication handle via each service call, because then we build a complex graph of dependency and communication for a single operation, we couple everything, and its hard to separate services and maintain.

So instead, this pkg wil:

- Orchestrate those cross service write operations (workflows)
- it has a DB since it can store state related to a workflow, as it has to compensate, retry, etc.
- Each domain exposes operations unaware of how they're being used, because the principle is that each domain operation should work as an isolated domain operation, each domain should only care about its own behavior and rules
- It can have logic, as long as its orchestration logic, no business logic. logic can be like knowing what next step to do depending on the response from the previous step
