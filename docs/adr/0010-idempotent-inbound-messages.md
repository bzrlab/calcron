# Idempotent inbound messages

Every state-changing command and signal includes an application-supplied idempotency key. Calcron stores the original result and returns it on repeated keys, so connection retries cannot create duplicate schedules, workflow instances, or transitions.

[ADR 0027](0027-bounded-finished-work-retention.md) bounds stored responses to
the configured retention window (30 days by default). Reusing an expired key
can execute the command again.
