# Idempotent inbound messages

Every state-changing command and signal includes an application-supplied idempotency key. Calcron stores the original result and returns it on repeated keys, so connection retries cannot create duplicate schedules, workflow instances, or transitions.
