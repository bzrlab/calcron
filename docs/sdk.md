# Calcron application SDK guide

Calcron is a durable scheduler and workflow engine for registered applications. The Node and Go SDKs authenticate over the application WebSocket endpoint, reconnect after transport loss, correlate requests with replies, and receive durable deliveries. They expose **application** operations only; workflow publishing, calendars, and recovery actions require an administrator.

Set `CALCRON_URL` to the WebSocket URL (for example, `ws://calcron.internal:8080/ws`) and load the one-time application token into `CALCRON_TOKEN`. Do not put a token in the URL or source control.

## Install and connect

Publish the Node package to your private registry, then install it with `npm install @calcron/node`. Go applications import `github.com/calcron/calcron/sdk/go` (package name `cron`).

The SDK authenticates during `connect` / `Connect`. A failed authentication returns an error; reconnect attempts use exponential backoff up to ten seconds. `close` / `Close` stops reconnecting. A request in flight when a connection is lost fails, so retry it with the **same idempotency key**.

## Application operations

Every state-changing operation requires an application-supplied idempotency key. Durations use Go duration syntax such as `30s`, `5m`, or `24h`. An absolute deadline uses an RFC 3339 UTC timestamp such as `2026-09-28T10:00:00Z`.

| Operation | What it does | Result |
| --- | --- | --- |
| `set` | Creates or replaces the application-scoped schedule named by `key`. Provide exactly one of `after` or `at`. | `scheduleId`, `runAt` |
| `cancel` | Cancels the named schedule; cancelling an absent schedule is safe. | `cancelled` |
| `extend` | Adds `by` to the current deadline, or from now if it is already due. | `runAt` |
| `throttle` | Immediately creates one delivery for `key`, then ignores repeated triggers during `cooldown` (leading edge). | `triggered` |
| `start` | Starts the newest published version of a named workflow. | `instanceId` |
| `signal` | Matches waiting workflows by both event and correlation key. | `matched` |

`set` and `throttle` accept a payload as `data`. A `chain` has `key`, `event`, `after`, and optional `data`; Calcron creates that successor only after the parent delivery is acknowledged. A chain is not business completion.

## Delivery and completion

Deliveries are **at least once**. The same delivery ID can arrive more than once after a disconnect or missed acknowledgement. Make the handler's durable effect idempotent by delivery ID or by the business object before calling `ack` / `Ack`.

Acknowledgement means only that the application received and durably handled a command. If a workflow waits for an external outcome, verify that outcome in the application and then send `signal` with the workflow's exact event and correlation key. Do not treat an acknowledgement as payment, email, or other business completion.

Register handlers before connecting when using Node, and immediately after `Connect` when using Go. An unacknowledged delivery remains eligible for retry.

## Idempotency keys

Use one stable, application-generated key for one intended state change:

- Reuse the same key only when retrying that exact command after a timeout, disconnect, or unknown reply.
- Use a new key for a new business intent, even if its payload is identical.
- Include the business ID and a version or intent label, for example `invoice:42:due:v1` or `invoice:42:paid:v1`.
- The SDK generates `ack:<deliveryId>` for an acknowledgement. Do not create a second acknowledgement key for the same delivery.

## Node / TypeScript example

```ts
import { Calcron } from "@calcron/node";

const calcron = new Calcron(process.env.CALCRON_URL!, process.env.CALCRON_TOKEN!);
calcron.on("invoice.due", async delivery => {
  await saveDeliveryReceipt(delivery.id, delivery.data); // idempotent transaction
  await delivery.ack();
});
await calcron.connect();
const schedule = await calcron.set({
  key: "invoice:42", event: "invoice.due", after: "24h", data: { invoiceId: 42 },
  chain: { key: "invoice:42:overdue", event: "invoice.overdue", after: "24h" },
  idempotencyKey: "invoice:42:due:v1",
});
const first = await calcron.throttle({ key: "invoice:42:notice", event: "invoice.notice", cooldown: "5m", idempotencyKey: "invoice:42:notice:v1" });
if (first.triggered) console.log(schedule.runAt);
await calcron.signal("payment.confirmed", "invoice:42", "invoice:42:paid:v1", { paid: true });
```

## Go example

```go
client, err := cron.Connect(ctx, os.Getenv("CALCRON_URL"), os.Getenv("CALCRON_TOKEN"))
if err != nil { return err }
defer client.Close()
client.On("invoice.due", func(event cron.Event) {
	if err := saveDeliveryReceipt(event.ID, event.Data); err != nil { return }
	_, _ = event.Ack(context.Background())
})
schedule, err := client.Set(ctx, cron.Schedule{Key: "invoice:42", Event: "invoice.due", After: "24h", Data: map[string]any{"invoiceId": 42}, IdempotencyKey: "invoice:42:due:v1"})
if err != nil { return err }
throttle, err := client.Throttle(ctx, cron.Throttle{Key: "invoice:42:notice", Event: "invoice.notice", Cooldown: "5m", IdempotencyKey: "invoice:42:notice:v1"})
if err != nil { return err }
if throttle.Triggered { log.Print(schedule.RunAt) }
_, err = client.Signal(ctx, "payment.confirmed", "invoice:42", "invoice:42:paid:v1", map[string]bool{"paid": true})
return err
```
