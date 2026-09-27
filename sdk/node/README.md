# @calcron/node

Node 22+ and TypeScript client for a Calcron application. It authenticates over
WebSocket, reconnects after a transport loss, correlates replies, and exposes
durable delivery acknowledgements.

```sh
npm install @calcron/node
```

```ts
import { Calcron } from "@calcron/node";

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

Read the [SDK guide](https://github.com/calcron/calcron/blob/main/docs/sdk.md)
for typed event contracts, every operation, delivery and idempotency rules, and
Go examples.
