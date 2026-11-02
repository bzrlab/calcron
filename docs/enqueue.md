# Immediate delivery with enqueue

Applications can send `enqueue` over `POST /commands` or an authenticated `/ws`
connection. It creates an ordinary durable schedule due now, without requiring a
schedule key or deadline. Delivery happens on the next delivery tick when a
WebSocket connection or registered HTTP callback is available.

```json
{"op":"enqueue","event":"order.deliver","data":{"orderId":"42"},"idempotencyKey":"order:42:deliver:v1"}
```

For HTTP, send `Authorization: Bearer <application token>` and
`Content-Type: application/json` to `/commands`. WebSocket applications send the
same frame after authentication. Both return the normal command envelope:

```json
{"ok":true,"data":{"scheduleId":"generated-id","runAt":"2026-11-02T10:00:00Z"}}
```

`event` is required. `data` is any JSON value and defaults to `{}` when omitted.
`idempotencyKey` is optional. With a key, concurrent or later retries within the
same application return the stored response and create one schedule, even if a
retry changes the event or payload. Keys share the existing application-wide
namespace with other commands. Use a distinct key for each business intent.

Without a key, every invocation creates a new schedule with a generated unique
schedule key. Retrying after an unknown response can therefore create another
job; supply a stable key whenever the caller may retry. Enqueue always creates
an immediate one-off schedule; scheduling, recurrence and chain fields belong
to `schedule.set` and are not used by enqueue.

Enqueue uses the existing at-least-once delivery, lease, acknowledgement,
exponential retry and blocked-delivery rules. A successful enqueue response
confirms durable acceptance, not delivery or business completion. Handlers must
make their effects idempotent and acknowledge successful deliveries. Offline
applications keep their work queued. After five failed connected attempts, an
administrator must replay or cancel the blocked delivery.

## Node

```ts
const job = await calcron.enqueue({
  event: "order.deliver", data: { orderId: "42" },
  idempotencyKey: "order:42:deliver:v1",
});
console.log(job.scheduleId, job.runAt);
```

`enqueue` uses the client's event map to check the event and payload, and returns
`ScheduleResult`. Omit `idempotencyKey` for an independent invocation.

## Go

```go
job, err := client.Enqueue(ctx, cron.Enqueue{
    Event: "order.deliver", Data: map[string]string{"orderId": "42"},
    IdempotencyKey: "order:42:deliver:v1",
})
```

For a typed event, use `cron.EnqueueTyped(ctx, client, cron.TypedEnqueue[Payload]{
Event: event, Data: payload, IdempotencyKey: key})`. Both return
`ScheduleResult, error`. Its `RunAt` is a `time.Time`. An empty `IdempotencyKey`
requests an independent invocation.
