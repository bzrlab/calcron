import assert from "node:assert/strict";
import test from "node:test";
import { Calcron, type Schedule } from "./index.ts";

type Frame = Record<string, unknown>;

class FakeWebSocket {
  static readonly OPEN = 1;
  static connections: FakeWebSocket[] = [];
  static onCreate: ((socket: FakeWebSocket) => void) | undefined;
  readyState = FakeWebSocket.OPEN;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  sent: Frame[] = [];
  respond: (frame: Frame) => void = frame => this.reply({ id: frame.id, ok: true, data: frame.op === "delivery.ack" ? { acked: true } : {} });

  constructor(_url: string) {
    FakeWebSocket.connections.push(this);
    FakeWebSocket.onCreate?.(this);
    queueMicrotask(() => this.onopen?.());
  }

  send(raw: string) {
    const frame = JSON.parse(raw) as Frame;
    this.sent.push(frame);
    this.respond(frame);
  }

  close() {
    this.readyState = 3;
    this.onclose?.();
  }

  reply(frame: Frame) { this.onmessage?.({ data: JSON.stringify(frame) }); }
}

function installFakeWebSocket() {
  FakeWebSocket.connections = [];
  FakeWebSocket.onCreate = undefined;
  globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket;
}

async function connected() {
  installFakeWebSocket();
  const client = new Calcron("ws://calcron.test/ws", "test");
  await client.connect();
  return { client, socket: FakeWebSocket.connections[0]! };
}

const schedule: Schedule = {
  key: "invoice:42",
  event: "invoice.due",
  after: "1h",
  data: { invoiceId: 42 },
  chain: { key: "invoice:42:overdue", event: "invoice.overdue", after: "24h" },
  idempotencyKey: "schedule:invoice:42:v1",
};

test("sends typed application commands and returns their replies", async () => {
  const { client, socket } = await connected();
  socket.respond = frame => {
    const data = frame.op === "schedule.set" ? { scheduleId: "s-1", runAt: "2026-09-28T10:00:00Z" }
      : frame.op === "schedule.throttle" ? { triggered: true }
      : frame.op === "schedule.cancel" ? { cancelled: true }
      : frame.op === "schedule.extend" ? { runAt: "2026-09-28T11:00:00Z" }
      : frame.op === "workflow.start" ? { instanceId: "w-1" }
      : frame.op === "signal" ? { matched: 1 }
      : {};
    socket.reply({ id: frame.id, ok: true, data });
  };

  assert.deepEqual(await client.set(schedule), { scheduleId: "s-1", runAt: "2026-09-28T10:00:00Z" });
  assert.deepEqual(await client.throttle({ ...schedule, cooldown: "5m" }), { triggered: true });
  assert.deepEqual(await client.cancel("invoice:42", "cancel:invoice:42:v1"), { cancelled: true });
  assert.deepEqual(await client.extend("invoice:42", "1h", "extend:invoice:42:v1"), { runAt: "2026-09-28T11:00:00Z" });
  assert.deepEqual(await client.start("invoice-lifecycle", "start:invoice:42:v1", { invoiceId: 42 }), { instanceId: "w-1" });
  assert.deepEqual(await client.signal("invoice.paid", "invoice:42", "signal:invoice:42:v1", { paid: true }), { matched: 1 });
  assert.deepEqual(socket.sent.map(frame => frame.op), ["auth", "schedule.set", "schedule.throttle", "schedule.cancel", "schedule.extend", "workflow.start", "signal"]);
  client.close();
});

test("acknowledges deliveries only when the handler calls ack", async () => {
  const { client, socket } = await connected();
  let delivery: { id: string; data: unknown; ack: () => Promise<unknown> } | undefined;
  client.on("invoice.due", event => { delivery = event; });
  socket.reply({ op: "delivery", deliveryId: "d-1", event: "invoice.due", data: { invoiceId: 42 } });
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(delivery?.data, { invoiceId: 42 });
  assert.equal(socket.sent.some(frame => frame.op === "delivery.ack"), false);
  assert.deepEqual(await delivery?.ack(), { acked: true });
  assert.deepEqual(socket.sent.at(-1), { id: "2", op: "delivery.ack", deliveryId: "d-1", idempotencyKey: "ack:d-1" });
  client.close();
});

test("surfaces protocol errors", async () => {
  const { client, socket } = await connected();
  socket.respond = frame => socket.reply({ id: frame.id, ok: false, error: "schedule not found" });
  await assert.rejects(client.cancel("missing", "cancel:missing:v1"), /schedule not found/);
  client.close();
});

test("correlates replies that arrive out of order", async () => {
  const { client, socket } = await connected();
  const pending: Frame[] = [];
  socket.respond = frame => pending.push(frame);
  const cancel = client.cancel("invoice:42", "cancel:invoice:42:v1");
  const extend = client.extend("invoice:42", "1h", "extend:invoice:42:v1");
  socket.reply({ id: pending[1]?.id, ok: true, data: { runAt: "2026-09-28T11:00:00Z" } });
  socket.reply({ id: pending[0]?.id, ok: true, data: { cancelled: true } });
  assert.deepEqual(await cancel, { cancelled: true });
  assert.deepEqual(await extend, { runAt: "2026-09-28T11:00:00Z" });
  client.close();
});

test("delivers duplicate delivery IDs to an idempotent handler", async () => {
  const { client, socket } = await connected();
  const ids: string[] = [];
  client.on("invoice.due", event => { ids.push(event.id); });
  socket.reply({ op: "delivery", deliveryId: "d-1", event: "invoice.due", data: {} });
  socket.reply({ op: "delivery", deliveryId: "d-1", event: "invoice.due", data: {} });
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(ids, ["d-1", "d-1"]);
  client.close();
});

test("rejects invalid credentials during connect", async () => {
  installFakeWebSocket();
  const client = new Calcron("ws://calcron.test/ws", "invalid");
  const connecting = client.connect();
  const socket = FakeWebSocket.connections[0]!;
  socket.respond = frame => socket.reply({ id: frame.id, ok: false, error: "bad token" });
  await assert.rejects(connecting, /bad token/);
  client.close();
});

test("reconnects after a transport loss", async () => {
  const { client, socket } = await connected();
  socket.close();
  await new Promise<void>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("no reconnect")), 1_000);
    const check = () => {
      if (FakeWebSocket.connections.length === 2) { clearTimeout(timer); resolve(); return; }
      setTimeout(check, 10);
    };
    check();
  });
  client.close();
});

test("stops and closes when authentication fails during reconnect", async () => {
  const { client, socket } = await connected();
  FakeWebSocket.onCreate = reconnecting => {
    if (FakeWebSocket.connections.length === 2) reconnecting.respond = frame => reconnecting.reply({ id: frame.id, ok: false, error: "bad token" });
  };
  socket.close();
  await new Promise(resolve => setTimeout(resolve, 300));
  assert.equal(FakeWebSocket.connections.length, 2, "invalid credentials retried unexpectedly");
  assert.equal(FakeWebSocket.connections[1]?.readyState, 3, "failed authentication left a live socket");
  client.close();
});
