/** A schedule created or replaced by `set`. Durations use Go duration syntax. */
type ScheduleBase = {
  key: string;
  event: string;
  data?: unknown;
  chain?: Chain;
  idempotencyKey: string;
};
export type Schedule = ScheduleBase & ({ after: string; at?: never } | { at: string; after?: never });

/** A successor scheduled only after its parent delivery is acknowledged. */
export type Chain = { key: string; event: string; after: string; data?: unknown };
/** A leading-edge immediate delivery, limited to one trigger per cooldown window. */
export type Throttle = ScheduleBase & { cooldown: string };
export type ScheduleResult = { scheduleId: string; runAt: string };
export type CancelResult = { cancelled: boolean };
export type ThrottleResult = { triggered: boolean };
export type WorkflowResult = { instanceId: string };
export type SignalResult = { matched: number };
export type AckResult = { acked: boolean };
export type Delivery = { id: string; data: unknown; ack: () => Promise<AckResult> };
type Reply = { id?: string; ok?: boolean; error?: string; op?: string; deliveryId?: string; event?: string; data?: unknown };

export class Calcron {
  private ws!: WebSocket;
  private n = 0;
  private waiting = new Map<string, { resolve: (v: unknown) => void; reject: (e: Error) => void }>();
  private handlers = new Map<string, (event: Delivery) => void | Promise<void>>();
  private stopped = false;
  private retry = 250;
  private reconnecting = false;
  private url: string;
  private token: string;
  constructor(url: string, token: string) { this.url = url; this.token = token; }
  async connect() { this.stopped = false; await this.open(); }
  close() { this.stopped = true; this.ws?.close(); }
  on(event: string, handler: (event: Delivery) => void | Promise<void>) { this.handlers.set(event, handler); }
  set(schedule: Schedule) { return this.request<ScheduleResult>({ op: "schedule.set", ...schedule }); }
  throttle(throttle: Throttle) { return this.request<ThrottleResult>({ op: "schedule.throttle", ...throttle }); }
  cancel(key: string, idempotencyKey: string) { return this.request<CancelResult>({ op: "schedule.cancel", key, idempotencyKey }); }
  extend(key: string, by: string, idempotencyKey: string) { return this.request<Pick<ScheduleResult, "runAt">>({ op: "schedule.extend", key, by, idempotencyKey }); }
  start(name: string, idempotencyKey: string, data?: unknown) { return this.request<WorkflowResult>({ op: "workflow.start", name, data, idempotencyKey }); }
  signal(event: string, correlationKey: string, idempotencyKey: string, data?: unknown) { return this.request<SignalResult>({ op: "signal", event, correlationKey, data, idempotencyKey }); }
  private async open() { this.ws = new WebSocket(this.url); await new Promise<void>((ok, bad) => { this.ws.onopen = () => ok(); this.ws.onerror = () => bad(new Error("Calcron connection failed")); }); this.ws.onmessage = e => this.receive(JSON.parse(e.data)); this.ws.onclose = () => this.lost(); try { await this.request({ op: "auth", token: this.token }); } catch (error) { this.stopped = true; this.ws.close(); throw error; } this.retry = 250; }
  private request<T>(body: Record<string, unknown>) { const id = String(++this.n); return new Promise<T>((resolve, reject) => { this.waiting.set(id, { resolve: value => resolve(value as T), reject }); if (this.ws.readyState !== WebSocket.OPEN) { this.waiting.delete(id); reject(new Error("Calcron disconnected")); return; } this.ws.send(JSON.stringify({ id, ...body })); }); }
  private receive(reply: Reply) { if (reply.op === "delivery") { const h = this.handlers.get(reply.event!); if (h) void h({ id: reply.deliveryId!, data: reply.data, ack: () => this.request<AckResult>({ op: "delivery.ack", deliveryId: reply.deliveryId, idempotencyKey: `ack:${reply.deliveryId}` }) }); return; } const p = this.waiting.get(reply.id!); if (!p) return; this.waiting.delete(reply.id!); reply.ok ? p.resolve(reply.data) : p.reject(new Error(reply.error ?? "Calcron request failed")); }
  private lost() { for (const [id, p] of this.waiting) { this.waiting.delete(id); p.reject(new Error("Calcron disconnected")); } if (this.stopped || this.reconnecting) return; this.reconnecting = true; const wait = this.retry; this.retry = Math.min(this.retry * 2, 10_000); setTimeout(() => void this.open().catch(() => {}).finally(() => { this.reconnecting = false; if (!this.stopped && this.ws.readyState !== WebSocket.OPEN) this.lost(); }), wait); }
}
