# Bounded retention of finished work and replay cache

Supersedes ADR 0023 for finished schedules, deliveries and their history. Measured
growth in #1 and the platform job runner in #14 justify automatic deletion after
30 days by default, configurable through `CALCRON_RETENTION` (#16).

Use bounded oldest-first cleanup on the existing tick rather than partitioning
or an admin purge command: each replica removes at most 100 rows per table per
minute, skips locked rows, applies separate pool-wait and transaction timeouts per table, and logs committed
counts. Each table commits independently so lock contention cannot starve the
other tables. Repeated failures log an ERROR with a consecutive-failure count. This bounds
interference with delivery processing while allowing a backlog to drain.

Preserve pending, blocked, leased, scheduled and awaiting-completion work. Keep
schedules while deliveries reference them, deliveries tied to unfinished workflows,
and history for subjects still present. Workflow, signal and start-schedule
history remains retained indefinitely. Terminal status changes
start a fresh delivery retention window; legacy deliveries receive a full window
at migration because cancellation timestamps were not recorded.

The idempotency replay cache expires independently by creation time. This bounds
ADR 0010's replay guarantee: reusing an expired key may execute the command again,
so operators must retain responses longer than applications might retry. Deletion
is permanent; retention is not an archive. See [operator configuration](../retention.md).
