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

## API reference

| SDK | Method | Parameters |
| --- | --- | --- |
| Node | `set(schedule)` | `key`, `event`, exactly one of `after` / `at`, optional `data` / `chain`, `idempotencyKey` → `ScheduleResult` |
| Node | `throttle(request)` | `key`, `event`, `cooldown`, optional `data` / `chain`, `idempotencyKey` → `ThrottleResult` |
| Node | `cancel(key, idempotencyKey)` / `extend(key, by, idempotencyKey)` | schedule key and mutation → `CancelResult` / `{ runAt }` |
| Node | `start(name, idempotencyKey, data?)` / `signal(event, correlationKey, idempotencyKey, data?)` | workflow start or correlated completion → `WorkflowResult` / `SignalResult` |
| Node | `on(event, handler)` | handler receives `{ id, data, ack() }`; `ack()` returns `AckResult` |
| Go | `Set(ctx, Schedule)` / `Throttle(ctx, Throttle)` | Same fields as Node; `Chain` is `{ Key, Event, After, Data }` → typed result and `error` |
| Go | `Cancel`, `Extend`, `Start`, `Signal` | Same positional fields as Node, prefixed with `context.Context` → typed result and `error` |
| Go | `On(event, func(Event))` | `Event` has `ID`, `Name`, `Data`; `Ack(ctx)` returns `AckResult, error` |

Node rejects connection/authentication and server failures as `Error`; a pending request rejects with `Calcron disconnected` if its socket is lost. Go returns those failures as `error`; `Set` also rejects a schedule that does not provide exactly one deadline. Server validation errors include missing required fields, malformed durations or timestamps, and unknown workflow or schedule names.

## Typed event contracts

Define your event schema once and give it to the SDK. TypeScript then rejects an
unknown event name, a wrong payload, a missing required payload, and a workflow
started with the wrong input before code runs.

```ts
import { Calcron } from "@calcron/node";

type Events = {
  "invoice.due": { invoiceId: string; amount: number };
  "invoice.paid": { invoiceId: string; paidAt: string };
};
type Workflows = { "invoice-lifecycle": { invoiceId: string } };

const calcron = new Calcron<Events, Workflows>(url, token);
calcron.on("invoice.due", delivery => delivery.data.amount); // number
await calcron.set({ key: "invoice:42", event: "invoice.due", after: "24h", data: { invoiceId: "42", amount: 100 }, idempotencyKey: "invoice:42:due:v1" });
await calcron.signal("invoice.paid", "invoice:42", "invoice:42:paid:v1", { invoiceId: "42", paidAt: new Date().toISOString() });
await calcron.start("invoice-lifecycle", "invoice:42:start:v1", { invoiceId: "42" });
```

Go binds each event-name value to its payload through generics. Declare the
value once, then use `OnTyped`, `SetTyped`, `ThrottleTyped`, and `SignalTyped`;
the compiler infers the payload from that value and `TypedDelivery` is already
decoded. `EventOf` is the Go equivalent of the TypeScript event map.

```go
type InvoiceDue struct { InvoiceID string `json:"invoiceId"`; Amount int `json:"amount"` }
var InvoiceDueEvent = cron.EventOf[InvoiceDue]("invoice.due")

cron.OnTyped(client, InvoiceDueEvent, func(delivery cron.TypedDelivery[InvoiceDue]) {
	_ = persistReceipt(delivery.ID, delivery.Data.Amount)
	_, _ = delivery.Ack(context.Background())
})
_, err := cron.SetTyped(ctx, client, cron.TypedSchedule[InvoiceDue]{
	Key: "invoice:42", Event: InvoiceDueEvent, After: "24h",
	Data: InvoiceDue{InvoiceID: "42", Amount: 100}, IdempotencyKey: "invoice:42:due:v1",
})
```

Keep raw `on` / `On`, `set` / `Set`, and `signal` / `Signal` only for gradual
migrations or dynamic event names. New application code should use the typed
surface.

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

type Events = {
  "invoice.due": { invoiceId: number };
  "invoice.overdue": { invoiceId: number };
  "invoice.notice": undefined;
  "payment.confirmed": { paid: boolean };
};
const calcron = new Calcron<Events>(process.env.CALCRON_URL!, process.env.CALCRON_TOKEN!);
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
type InvoiceDue struct { InvoiceID int `json:"invoiceId"` }
var InvoiceDueEvent = cron.EventOf[InvoiceDue]("invoice.due")
var InvoiceNoticeEvent = cron.EventOf[struct{}]("invoice.notice")
var PaymentConfirmed = cron.EventOf[struct{ Paid bool `json:"paid"` }]("payment.confirmed")
cron.OnTyped(client, InvoiceDueEvent, func(event cron.TypedDelivery[InvoiceDue]) {
	if err := saveDeliveryReceipt(event.ID, event.Data); err != nil { return }
	_, _ = event.Ack(context.Background())
})
schedule, err := cron.SetTyped(ctx, client, cron.TypedSchedule[InvoiceDue]{Key: "invoice:42", Event: InvoiceDueEvent, After: "24h", Data: InvoiceDue{InvoiceID: 42}, IdempotencyKey: "invoice:42:due:v1"})
if err != nil { return err }
throttle, err := cron.ThrottleTyped(ctx, client, cron.TypedThrottle[struct{}]{Key: "invoice:42:notice", Event: InvoiceNoticeEvent, Cooldown: "5m", Data: struct{}{}, IdempotencyKey: "invoice:42:notice:v1"})
if err != nil { return err }
if throttle.Triggered { log.Print(schedule.RunAt) }
_, err = cron.SignalTyped(ctx, client, PaymentConfirmed, "invoice:42", "invoice:42:paid:v1", struct{ Paid bool `json:"paid"` }{Paid: true})
return err
```
