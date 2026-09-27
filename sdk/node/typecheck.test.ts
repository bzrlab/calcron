import { Calcron, type Delivery, type Schedule } from "./index.js";

type AppEvents = {
  "invoice.due": { invoiceId: string; amount: number };
  "invoice.paid": { invoiceId: string; paidAt: string };
};
type Workflows = { "invoice-lifecycle": { invoiceId: string } };

declare const calcron: Calcron<AppEvents, Workflows>;

const due: Schedule<AppEvents, "invoice.due"> = {
  key: "invoice:42",
  event: "invoice.due",
  after: "24h",
  data: { invoiceId: "42", amount: 100 },
  idempotencyKey: "invoice:42:due:v1",
};

const typedDelivery: Delivery<AppEvents, "invoice.due"> = {
  id: "d-1",
  data: { invoiceId: "42", amount: 100 },
  ack: async () => ({ acked: true }),
};

calcron.on("invoice.due", async delivery => {
  const invoiceID: string = delivery.data.invoiceId;
  await delivery.ack();
  void invoiceID;
});
void calcron.set(due);
void calcron.start("invoice-lifecycle", "invoice:42:start:v1", { invoiceId: "42" });
void typedDelivery;

// @ts-expect-error event data must match the event name.
void calcron.signal("invoice.paid", "invoice:42", "invoice:42:paid:v1", { amount: 100 });
// @ts-expect-error workflow input must match the workflow name.
void calcron.start("invoice-lifecycle", "invoice:42:start:v2", { amount: 100 });
