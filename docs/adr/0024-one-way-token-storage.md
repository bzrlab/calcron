# One-way token storage

Calcron displays an administrator or application token only when created, then retains only a salted hash. Clients load tokens from environment variables, and token rotation supports an overlap period so applications need not stop.
