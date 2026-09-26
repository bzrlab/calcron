# Calcron: generic durable workflow engine

## Problem Statement

The user's applications each need future work, expiry, retries, inactivity handling, recurring business-day actions, and recovery after outages. Rebuilding timers, polling loops, and lifecycle state in every application causes duplicate logic and lost or unclear work. Some work is financial and must not silently run twice, target the wrong business day, or disappear while an application is offline.

Calcron must provide one private service that applications build around. It must remain generic. It must not contain Discord, MFS, payment, database, or other application-specific behavior.

## Solution

Build Calcron as one Go service with PostgreSQL durability and an authenticated WebSocket protocol. Calcron runs declarative, versioned workflows and direct scheduling operations. Applications connect over WebSocket, receive durable commands, acknowledge receipt, and send correlated signals when outside work completes.

The workflow interpreter supports time waits, signal waits, CEL branches, command emission, and completion. Named business calendars calculate valid local business times. PostgreSQL stores all state and history. A single real-time dashboard lets administrators inspect and recover work.

## User Stories

1. As an application developer, I want one private scheduling service, so that I do not build timers and polling loops in each application.
2. As an application developer, I want to create a named schedule with a relative or absolute deadline, so that future work has a durable deadline.
3. As an application developer, I want `set` to replace a schedule with the same key, so that ticket activity can reset an inactivity deadline.
4. As an application developer, I want cancellation to be idempotent, so that retried cancellation requests are safe.
5. As an application developer, I want to extend a deadline, so that I can add time without changing the original schedule semantics.
6. As an application developer, I want chains to start after command acknowledgement, so that a failed parent command does not advance work.
7. As an application developer, I want leading-edge throttling, so that one event can be delivered immediately while repeated events in a cooldown window do nothing.
8. As an application developer, I want applications to communicate through one authenticated WebSocket endpoint, so that I do not expose webhook servers for internal services.
9. As an application runtime, I want due commands queued while I am disconnected, so that outages do not lose work.
10. As an application runtime, I want delivery retries after a failed acknowledgement, so that temporary failures recover.
11. As an application developer, I want at-least-once delivery, so that durable delivery does not make impossible exactly-once claims.
12. As an application developer, I want a stable delivery ID, so that my handler can safely process duplicate delivery.
13. As an application developer, I want an acknowledgement to mean command receipt only, so that business completion stays explicit.
14. As an application developer, I want to send a correlated completion signal, so that a workflow advances only after my application verifies an outside effect.
15. As an application developer, I want every state-changing command and signal to have an idempotency key, so that reconnects cannot duplicate state changes.
16. As an administrator, I want to register applications and scoped tokens, so that one application cannot alter another application's work.
17. As an administrator, I want tokens shown only once and stored as salted hashes, so that database disclosure does not disclose usable credentials.
18. As an administrator, I want token rotation overlap, so that applications can rotate credentials without downtime.
19. As an administrator, I want to publish a workflow definition, so that generic lifecycle behavior does not live in application code.
20. As an administrator, I want published definitions to be immutable versions, so that existing workflow histories stay reproducible.
21. As an application developer, I want to start an authorised workflow, so that application events can initiate a lifecycle.
22. As an application developer, I want workflow instances to retain their own input, state, and history, so that concurrent work stays isolated.
23. As an application developer, I want a workflow to wait until a time, so that delayed work does not require an active application process.
24. As an application developer, I want a workflow to wait for a named correlated signal, so that a receipt cannot advance the wrong workflow.
25. As an application developer, I want CEL conditions and calculations over workflow data, so that workflows branch on generic facts without running application code inside Calcron.
26. As an application developer, I want CEL restricted to read-only evaluation, so that workflow definitions cannot access networks, databases, imports, or side effects.
27. As an application developer, I want a workflow to emit a command to a registered application, so that Calcron can orchestrate effects without owning integrations.
28. As an application developer, I want direct schedule operations separate from workflow definitions, so that simple expiry use cases stay simple.
29. As an administrator, I want named business calendars with IANA timezones, so that workflows use valid local dates.
30. As an administrator, I want calendars to support weekly eligibility and explicit date overrides, so that different counterparties can have different weekend and exception rules.
31. As an application developer, I want a workflow to wait for the next eligible calendar time, so that business-day work does not run on excluded dates.
32. As an administrator, I want recurring start schedules that create workflow instances on calendar-aware times, so that daily business processes do not require application cron jobs.
33. As an administrator, I want every recurring start schedule to choose a missed-occurrence policy, so that downtime never silently skips or duplicates financial work.
34. As an administrator, I want to choose skip, one late run, or full catch-up, so that each recurrence matches its business risk.
35. As an MFS settlement application, I want Calcron to ask me to perform a settlement, so that the scheduler remains independent of transfer APIs and ledgers.
36. As an MFS settlement application, I want to report confirmed, partial, reversed, or failed outcomes through signals, so that generic workflows can branch without Calcron owning money logic.
37. As an operator, I want a real-time dashboard, so that I can see applications, schedules, workflows, deliveries, calendars, and history in one place.
38. As an operator, I want to inspect a workflow's full state history, so that I can diagnose why it waited, branched, retried, or stopped.
39. As an operator, I want to cancel, retry, and replay work from the dashboard, so that blocked delivery can be recovered safely.
40. As an operator, I want blocked deliveries after capped retries, so that persistent failures become visible instead of retrying forever.
41. As an operator, I want offline applications distinguished from failed delivery, so that a normal disconnection does not create false failure alerts.
42. As an administrator, I want durable append-only operational history in V1, so that later inspection has complete evidence.
43. As an application developer, I want a Node/TypeScript SDK, so that Discord and other Node applications use Calcron without raw WebSocket management.
44. As an application developer, I want a Go SDK, so that Go applications use the same reliable protocol behavior.
45. As an application developer, I want SDKs to manage authentication, reconnects, request replies, acknowledgements, and idempotency keys, so that application handlers stay focused on business behavior.
46. As an operator, I want a single containerised Calcron service, so that deployment stays small.
47. As an operator, I want local Docker Compose with PostgreSQL, so that local development resembles production without extra infrastructure.
48. As an operator, I want Calcron to use bounded PostgreSQL connections and sleep until work is due, so that idle CPU and memory stay low.
49. As an operator, I want multiple service instances to safely lease due work later, so that scale-out does not duplicate dispatch.
50. As an administrator, I want no built-in Discord, MFS, payment, or HTTP adapter, so that Calcron remains useful to every application.

## Implementation Decisions

- Calcron is private infrastructure for the owner's applications. It has no user accounts, organisations, billing, public application registration, or multi-tenant model in V1.
- Calcron is a generic workflow engine with scheduling. MFS settlement is a test case only. External applications own financial balances, transfer calls, ledgers, reconciliation, and business truth.
- One Go binary serves the dashboard at `/`, the WebSocket endpoint at `/ws`, and health check at `/health`. Dashboard assets are embedded in the binary.
- PostgreSQL is the sole durable store. Normal requests use a bounded connection pool. One dedicated connection receives database wakeups. Due work uses PostgreSQL leasing and can use `SKIP LOCKED` when multiple instances exist.
- V1 has no Redis, message broker, Kubernetes requirement, separate worker process, webhook endpoint, REST application API, raw cron syntax, or app-specific integration.
- Applications authenticate inside a WebSocket frame. URLs never contain tokens. Application credentials identify exactly one application and namespace. Administrator credentials are separate.
- Application tokens may create, alter, signal, and receive their authorised work. Only administrator credentials publish workflow definitions and business calendars.
- Tokens are displayed once, retained as salted hashes, loaded by clients from environment variables, and support overlapping rotation.
- Direct scheduler operations are `set`, `cancel`, `extend`, `chain`, and leading-edge `throttle`. `set` atomically creates or replaces by schedule key. `cancel` is idempotent. `extend` adds to existing deadline, or from current time after the deadline is due.
- A chain creates its successor only after its parent delivery receives an acknowledgement.
- Delivery is at least once. Disconnected applications retain pending delivery without failure count. Failed acknowledgements use capped exponential retry. Five failed attempts create a blocked delivery that an administrator must replay or cancel.
- Acknowledgement means only that an application received a command. An application emits a separate correlated signal to report actual business completion.
- Every state-changing command and signal has an application-supplied idempotency key. Repeated keys return the original result and do not make duplicate state changes.
- Workflow definitions are schema-validated JSON documents. Publishing creates an immutable version. New instances use the newest published version; existing instances never migrate in V1.
- Workflow definitions are finite-state machines. V1 states are `wait_time`, `wait_signal`, `branch`, `emit`, and `end`. Explicit transitions model loops.
- Conditions and derived values use restricted read-only CEL over workflow input, state, and signals.
- Every signal wait has an event name and correlation key. Incoming signals present the same pair. Calcron never infers business relationships from opaque payload data.
- Named business calendars belong to an application namespace. They contain IANA timezone, eligible weekdays, and date overrides. V1 does not fetch external holiday data.
- Named start schedules create a new workflow instance at each eligible calendar time. Each declares `skip`, `run_once_late`, or `catch_up` for missed occurrences. There is no default.
- Workflow commands specify target application, event name, and CEL-derived payload. Calcron does not call HTTP APIs, databases, payment providers, or Discord directly.
- V1 ships Node/TypeScript and Go SDKs. They own protocol authentication, reconnection, request correlation, command acknowledgement, and idempotency-key support.
- Operational history is append-only and retained indefinitely in V1. Archive or retention policy follows measured database growth.
- The single dashboard is operational. It provides live inspection plus cancel, retry, and replay. It has no workflow-definition editor in V1.
- The initial module boundaries are server and protocol, authentication and applications, direct scheduler, workflow interpreter, calendars, delivery, PostgreSQL store and migrations, dashboard, Node SDK, Go SDK, and deployment assets.

## Testing Decisions

- Primary test seam is the external WebSocket protocol against a real temporary PostgreSQL database. Tests must assert observable schedule, workflow, delivery, acknowledgement, signal, dashboard-query, and recovery behavior rather than private implementation functions.
- No test prior art exists because the repository has no application code yet. Establish protocol integration tests as the initial standard.
- Test an application client that connects, authenticates, creates work, receives commands, acknowledges them, disconnects, reconnects, and sends signals. This one seam exercises storage, scheduler, workflow interpreter, and delivery together.
- Test `set`, cancellation, extension, chains, and throttle through protocol responses and later received deliveries.
- Test duplicate state-changing messages with the same idempotency key. Assert the original result returns and only one state transition exists.
- Test delivery at-least-once behavior by disconnecting before acknowledgement and asserting the same delivery ID returns after reconnection.
- Test retry and blocked behavior with a connected client that does not acknowledge. Assert offline queued work does not increase failed attempts.
- Test command acknowledgement separately from completion signal. Assert a workflow does not advance from a business wait until matching signal name and correlation key arrive.
- Test correlation isolation with concurrent workflow instances that wait for the same event name and different keys.
- Test CEL conditions using accepted data types, true and false branches, invalid expressions, and restricted evaluation boundaries.
- Test business-calendar calculation across timezone boundaries, excluded weekdays, date overrides, and daylight-saving changes.
- Test each missed-occurrence policy after simulated downtime.
- Test workflow publication validation, immutable versions, and instance pinning.
- Test application permission boundaries and administrator-only publishing.
- Test token authentication with valid, revoked, and rotating credentials. Never log token values in test failures.
- Test dashboard data through its WebSocket subscription and command interface. Browser automation can verify only visible live updates and recovery controls after the protocol behavior is covered.
- Run PostgreSQL query-plan checks for due-work and pending-delivery queries before performance work. Indexes must match actual filtering, including partial indexes for active work and indexes on foreign keys.

## Out of Scope

- Multi-tenancy, public self-service signup, billing, organisations, and user accounts.
- Application-specific MFS, payment, Discord, HTTP, database, or queue adapters.
- Direct money movement, financial ledger ownership, reconciliation rules, or settlement amount calculation.
- Arbitrary Go, JavaScript, or user-supplied code execution inside Calcron.
- REST application API and incoming HTTP webhook delivery.
- Raw cron syntax, external holiday-provider integration, and in-browser workflow editor.
- Live workflow-instance migration after definition publication.
- SDKs beyond Node/TypeScript and Go.
- Automatic history pruning, archival, or external log shipping.
- Redis, RabbitMQ, NATS, Kafka, Kubernetes, and separate worker infrastructure.

## Further Notes

- Applications must make delivery handlers idempotent. Calcron can make dispatch durable but cannot make an outside system's side effect exactly once.
- For financial flows, business completion must come from an application signal after that application validates its own ledger or provider response.
- Workflows that command several applications are published only by an administrator. This keeps orchestration authority outside individual application tokens.
- Dashboard history is an operational audit trail, not a substitute for an application's financial ledger.
- Existing project language and ADRs in `CONTEXT.md` and `docs/adr/` govern this specification.
