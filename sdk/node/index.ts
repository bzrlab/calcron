export type Schedule = { key: string; event: string; after?: string; at?: string; data?: unknown; chain?: unknown; idempotencyKey: string };
type Reply = { id?: string; ok?: boolean; error?: string; op?: string; deliveryId?: string; event?: string; data?: unknown };

export class Calcron {
  private ws!: WebSocket;
  private n = 0;
  private waiting = new Map<string, { resolve: (v: unknown) => void; reject: (e: Error) => void }>();
  private handlers = new Map<string, (event: { id: string; data: unknown; ack: () => Promise<void> }) => void | Promise<void>>();
  private stopped = false;
  private retry = 250;
  private reconnecting = false;
  private url: string;
  private token: string;
  constructor(url: string, token: string) { this.url = url; this.token = token; }
  async connect() { this.stopped = false; await this.open(); }
  close() { this.stopped = true; this.ws?.close(); }
  on(event: string, handler: (event: { id: string; data: unknown; ack: () => Promise<void> }) => void | Promise<void>) { this.handlers.set(event, handler); }
  set(schedule: Schedule) { return this.request({ op: "schedule.set", ...schedule }); }
  cancel(key: string, idempotencyKey: string) { return this.request({ op: "schedule.cancel", key, idempotencyKey }); }
  extend(key: string, by: string, idempotencyKey: string) { return this.request({ op: "schedule.extend", key, by, idempotencyKey }); }
  start(name: string, idempotencyKey: string, data?: unknown) { return this.request({ op: "workflow.start", name, data, idempotencyKey }); }
  signal(event: string, correlationKey: string, idempotencyKey: string, data?: unknown) { return this.request({ op: "signal", event, correlationKey, data, idempotencyKey }); }
  private async open() { this.ws = new WebSocket(this.url); await new Promise<void>((ok, bad) => { this.ws.onopen = () => ok(); this.ws.onerror = () => bad(new Error("Calcron connection failed")); }); this.ws.onmessage = e => this.receive(JSON.parse(e.data)); this.ws.onclose = () => this.lost(); await this.request({ op: "auth", token: this.token }); this.retry = 250; }
  private request(body: Record<string, unknown>) { const id = String(++this.n); return new Promise<unknown>((resolve, reject) => { this.waiting.set(id, { resolve, reject }); if (this.ws.readyState !== WebSocket.OPEN) { this.waiting.delete(id); reject(new Error("Calcron disconnected")); return; } this.ws.send(JSON.stringify({ id, ...body })); }); }
  private receive(reply: Reply) { if (reply.op === "delivery") { const h = this.handlers.get(reply.event!); if (h) void h({ id: reply.deliveryId!, data: reply.data, ack: () => this.request({ op: "delivery.ack", deliveryId: reply.deliveryId, idempotencyKey: `ack:${reply.deliveryId}` }) }); return; } const p = this.waiting.get(reply.id!); if (!p) return; this.waiting.delete(reply.id!); reply.ok ? p.resolve(reply.data) : p.reject(new Error(reply.error)); }
  private lost() { for (const [id, p] of this.waiting) { this.waiting.delete(id); p.reject(new Error("Calcron disconnected")); } if (this.stopped || this.reconnecting) return; this.reconnecting = true; const wait = this.retry; this.retry = Math.min(this.retry * 2, 10_000); setTimeout(() => void this.open().catch(() => {}).finally(() => { this.reconnecting = false; if (!this.stopped && this.ws.readyState !== WebSocket.OPEN) this.lost(); }), wait); }
}
