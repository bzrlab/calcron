# @bzrlab/calcron

Node 22+ and TypeScript client for a Calcron application. It authenticates over
WebSocket, reconnects after a transport loss, correlates replies, and exposes
durable delivery acknowledgements.

```sh
npm install @bzrlab/calcron
```

```ts
import { Calcron } from "@bzrlab/calcron";

type Events = { "invoice.due": { invoiceId: string } };
const calcron = new Calcron<Events>(process.env.CALCRON_URL!, process.env.CALCRON_TOKEN!);
calcron.on("invoice.due", async delivery => {
  await persistReceipt(delivery.id, delivery.data); // must be idempotent
  await delivery.ack();
});
await calcron.connect();
await calcron.set({
  key: "invoice:42",
  event: "invoice.due",
  after: "24h",
  idempotencyKey: "invoice:42:due:v1",
});
```

Read the [SDK guide](https://github.com/bzrlab/calcron/blob/main/docs/sdk.md)
for typed event contracts, every operation, delivery and idempotency rules, and
Go examples.

## Immediate delivery

Use `await calcron.enqueue({ event: "invoice.due", data: { invoiceId: 42 }, idempotencyKey: "invoice:42:deliver:v1" })`. The event map checks the payload.

Returns `scheduleId` and `runAt` (`ScheduleResult` in Go). The idempotency key is optional; omitting it creates a new immediate schedule on every invocation. Supply a stable key for safe retries. Delivery uses the usual retry and acknowledgement rules. See [the command guide](https://github.com/bzrlab/calcron/blob/main/docs/enqueue.md).

## Connection concurrency

Use `new Calcron(url, token, { maxInFlight: 4 })` to cap distinct unacknowledged
deliveries on this connection. Omitted or zero means unlimited. The cap is
retained on reconnect; retries reuse a slot and still require idempotent handlers.
See [delivery cap semantics](https://github.com/bzrlab/calcron/blob/main/docs/concurrency.md).
