# Append-only history in V1

Partially superseded by [ADR 0027](0027-bounded-finished-work-retention.md) for
finished schedules, deliveries, their history, and the idempotency replay cache.

Calcron retains workflow, schedule, signal, and delivery history indefinitely in V1. It performs no automatic deletion or archive process; a later retention feature will respond to measured database growth rather than speculation.
