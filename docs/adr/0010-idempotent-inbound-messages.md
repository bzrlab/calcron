# Idempotent inbound messages

Every state-changing command and signal includes an application-supplied idempotency key. Calcron stores the original result and returns it on repeated keys, so connection retries cannot create duplicate schedules, workflow instances, or transitions.

## Enqueue exception (platform runner spec #14)

`enqueue` accepts an optional caller-supplied key. When omitted, Calcron generates
a unique key before entering the same application-scoped advisory-lock and
stored-response path. Each keyless invocation creates an independent immediate
schedule; a caller that may retry must supply a stable key. Other state-changing
commands still require a caller-supplied key.
