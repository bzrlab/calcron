# Payload limits

`CALCRON_MAX_PAYLOAD_BYTES` sets the maximum UTF-8 byte length of each JSON
`data` or `chain` field. The default is 65536 bytes (64 KiB). Embedded Go servers
set `server.Config.MaxPayloadBytes`; zero selects the default. Environment
values must be positive integers; invalid values stop startup. Negative or
unrepresentable Go configuration limits also fail startup.

The size includes JSON quotes, object keys, escaping and whitespace inside the
field, not just string contents. Exactly the limit is accepted. The complete
`chain` object must fit, including its nested payload. Each field is checked
independently. Keep large documents elsewhere and carry their identifiers.

The shared command router checks these fields for every application and admin
command, including enqueue, schedules, chains, delivery acknowledgements,
workflow starts, signals, workflow publishing, calendars and start schedules.
A field over the limit returns `{"ok":false,"error":"payload too large"}`,
with the command's `id` when supplied. HTTP returns status 200 for this command
error; WebSocket keeps the connection open. Rejection happens before side
effects and idempotency storage. A rejected command can be corrected and retried
with the same key. Oversized retries are rejected even when the key already has
a stored response; ordinary accepted retries retain their existing behavior.

Both transports also bound the **whole command** to the configured payload
limit plus 65536 envelope bytes. This includes unknown fields and whitespace,
and applies after WebSocket authentication. HTTP returns status 413
with the same `payload too large` JSON error (after authentication). WebSocket
closes with status 1009 (`Message Too Big`) and reason `payload too large`;
there is no command reply or correlation ID for this transport error. Reconnect
and reduce the command before retrying. HTTP and WebSocket read only a bounded
body before rejecting it.

These are inbound command limits. They do not truncate existing stored data,
limit dashboard replies, or change workflow execution and delivery semantics.
Changing the setting affects new command submissions, including payloads that
will later be used by recurring start schedules.

The Go SDK defaults to a 64 KiB expected delivery payload plus 64 KiB for the
JSON envelope. When increasing the server limit, also set the Go connection's
`cron.ConnectOptions{MaxPayloadBytes: ...}` to the expected payload limit. This
receive setting applies to the initial socket and every reconnect. Allow for
larger existing stored deliveries when lowering server submission limits.

WebSocket authentication frames have a separate 4 KiB limit, independent of the
configured payload size. The full command budget applies after authentication.
