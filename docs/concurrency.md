# Per-connection delivery cap

A WebSocket worker may declare `maxInFlight` in its initial authentication frame:

```json
{"op":"auth","token":"cc_...","maxInFlight":4}
```

The value is a non-negative integer. Omit it, or use `0`, for unlimited delivery
(the existing default). A negative value rejects authentication. The setting
belongs to this connection; it does not change the application or HTTP commands.

Calcron sends at most this many **distinct unacknowledged deliveries** to the
connection. Retries of an already-held delivery reuse its slot and keep their
existing delay, lease and retry limit. A retry can therefore invoke a handler
again before an earlier invocation finishes: handlers must remain idempotent.
This cap does not count handler invocations or guarantee exactly-once execution.

An acknowledgement committed through WebSocket or HTTP frees the slot on the
next delivery tick. Failed, unrelated, or cached acknowledgements do not free
another delivery's slot. Database failures leave accounting unchanged. A blocked
delivery remains unacknowledged and keeps its slot: replay it to retry in the
same slot, or cancel it to release the slot. Cancellation also frees a pending
delivery's slot. Lease expiry alone does not release a slot.

Connections for the same application have independent caps. A retry stays with
its assigned live connection on the same server. Disconnecting discards that
connection's accounting; its pending deliveries remain durable and can be sent
to a replacement after their normal retry/lease delay. A reconnect is a new
connection and must declare the cap again. Both SDKs do this automatically.
The cap is advisory per connection, not a distributed application-wide limit.

## Node / TypeScript

```ts
const client = new Calcron(url, token, { maxInFlight: 4 });
await client.connect();
```

## Go

```go
client, err := cron.Connect(ctx, url, token, cron.ConnectOptions{MaxInFlight: 4})
```

Existing two-argument Node constructors and three-argument Go `Connect` calls
continue to use unlimited delivery. Go accepts at most one options value.
