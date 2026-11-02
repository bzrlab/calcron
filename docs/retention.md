# Retention

Calcron automatically removes finished work and replay responses older than
30 days. Set `CALCRON_RETENTION=2160h` for 90 days, for example. Values are
positive Go durations (hours, minutes, seconds; `30d` is not supported). Unset or
invalid values use `720h`. Restart Calcron after changing the environment.

The delivery tick deletes up to 100 eligible rows per table per replica:
acknowledged/cancelled deliveries, unreferenced delivered/cancelled schedules,
their old history, and expired idempotency responses. Nonempty batches log
`retention cleanup: deliveries=... schedules=... history=... idempotency=...`.
Failures roll back and are logged; subsequent ticks retry. Cleanup uses a 50ms
lock timeout, a 200ms statement timeout and a one-second overall budget to bound
interference with delivery processing.

Pending, blocked, leased, scheduled, and awaiting-completion work stays. History
for subjects still present stays, as do workflow, signal and start-schedule
history. Cancellation starts a fresh retention window, regardless of creation
age. Existing deliveries receive a full window when this migration first runs.
Locked rows are skipped; backlog may take several ticks to drain.

After an idempotency response expires, reusing its key can execute the command
again. Keep retention longer than the application's maximum retry horizon.
Deletion is permanent; retain database backups if historical recovery is needed.
See [ADR 0027](adr/0027-bounded-finished-work-retention.md) for the safety rules.
