// Calcron's operator command catalog. This is the dashboard's protocol seam:
// views name an intention and receive its declared result, while AdminClient
// owns framing, correlation, authentication, and transport failures.
export type DashboardListName = "apps" | "schedules" | "deliveries" | "workflows" | "history" | "calendars" | "tokens" | "start_schedules" | "workflow_versions";
type DashboardListRequest = { name: DashboardListName; subjectType?: string; subjectId?: string; before?: number };

type CommandMap = {
  "app.create": { request: { name: string; namespace: string }; result: { applicationId: string; token: string } };
  "app.token.rotate": { request: { applicationId: string }; result: { tokenId: string; token: string } };
  "app.token.revoke": { request: { tokenId: string }; result: { revoked: boolean } };
  "workflow.publish": { request: { applicationId: string; name: string; data: unknown }; result: { version: number } };
  "calendar.set": { request: { applicationId: string; name: string; data: unknown }; result: unknown };
  "calendar.next": { request: { applicationId?: string; calendar?: string; data?: unknown; at: string }; result: { next: string } };
  "calendar.occurrences": { request: { applicationId?: string; calendar?: string; data?: unknown; localTime: string; at: string; until: string }; result: { occurrences: string[] } };
  "start-schedule.set": { request: { applicationId: string; name: string; workflow: string; calendar: string; localTime: string; missedPolicy: string; data?: unknown }; result: unknown };
  "dashboard.stats": { request: Record<never, never>; result: unknown };
  "dashboard.list": { request: DashboardListRequest; result: unknown[] };
  "delivery.replay": { request: { deliveryId: string }; result: { replayed: boolean } };
  "delivery.cancel": { request: { deliveryId: string }; result: { cancelled: boolean } };
};

export type AdminCommand = { [Op in keyof CommandMap]: { op: Op } & CommandMap[Op]["request"] }[keyof CommandMap];
export type CommandResult<C extends AdminCommand> = C extends { op: infer Op extends keyof CommandMap } ? CommandMap[Op]["result"] : never;
export type Reply<T = unknown> =
  | { id?: string; ok: true; data: T }
  | { id?: string; ok: false; error: string };
type AuthCommand = { op: "auth"; token: string };

export type DeliveryPush = {
  op: "delivery";
  deliveryId: string;
  scheduleId?: string;
  event: string;
  data?: unknown;
};

export type ConnState = "idle" | "connecting" | "open" | "closed";

type Pending = { resolve: (r: Reply) => void; reject: (e: Error) => void };

export class AuthRejected extends Error {}

export class AdminClient {
  private ws: WebSocket | null = null;
  private n = 0;
  private pending = new Map<string, Pending>();
  private url: string;
  private token: string;
  private onDelivery?: (d: DeliveryPush) => void;
  private onState?: (s: ConnState) => void;

  constructor(url: string, token: string) {
    this.url = url;
    this.token = token;
  }

  setHandlers(h: {
    onDelivery?: (d: DeliveryPush) => void;
    onState?: (s: ConnState) => void;
  }) {
    this.onDelivery = h.onDelivery;
    this.onState = h.onState;
  }

  connect(): Promise<void> {
    this.onState?.("connecting");
    this.ws = new WebSocket(this.url);
    return new Promise<void>((resolve, reject) => {
      const ws = this.ws!;
      ws.onopen = () => {
        this.request({ op: "auth", token: this.token })
          .then((r) => {
            if (!r.ok) {
              this.onState?.("closed");
              reject(new AuthRejected(r.error ?? "unauthorized"));
              return;
            }
            this.onState?.("open");
            resolve();
          })
          .catch(reject);
      };
      ws.onerror = () => reject(new Error("Calcron connection failed"));
      ws.onmessage = (e) => this.receive(JSON.parse(e.data as string));
      ws.onclose = () => {
        this.failPending(new Error("Calcron disconnected"));
        this.onState?.("closed");
      };
    });
  }

  close() {
    this.ws?.close();
    this.ws = null;
  }

  // request sends one admin frame and resolves with the reply. `data` carries the
  // op payload (definition, calendar, etc.); `idempotencyKey` is not required for
  // admin ops but is forwarded when present.
  request<C extends AdminCommand | AuthCommand>(
    body: C,
    timeoutMs = 10_000,
  ): Promise<Reply<C extends AdminCommand ? CommandResult<C> : { application: string; admin: boolean }>> {
    const id = String(++this.n);
    return new Promise<Reply<C extends AdminCommand ? CommandResult<C> : { application: string; admin: boolean }>>((resolve, reject) => {
      if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
        reject(new Error("Calcron disconnected"));
        return;
      }
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`timeout: ${body.op}`));
      }, timeoutMs);
      this.pending.set(id, {
        resolve: (r) => {
          clearTimeout(timer);
          resolve(r as Reply<C extends AdminCommand ? CommandResult<C> : { application: string; admin: boolean }>);
        },
        reject: (e) => {
          clearTimeout(timer);
          reject(e);
        },
      });
      try {
        this.ws.send(JSON.stringify({ id, ...body }));
      } catch (e) {
        // The socket can close between the readyState check above and send.
        clearTimeout(timer);
        this.pending.delete(id);
        reject(e instanceof Error ? e : new Error("send failed"));
      }
    });
  }

  private receive(raw: Reply | DeliveryPush) {
    if ((raw as DeliveryPush).op === "delivery") {
      this.onDelivery?.(raw as DeliveryPush);
      return;
    }
    const r = raw as Reply;
    const p = this.pending.get(r.id ?? "");
    if (!p) return;
    this.pending.delete(r.id ?? "");
    p.resolve(r);
  }

  private failPending(err: Error) {
    for (const [id, p] of this.pending) {
      this.pending.delete(id);
      p.reject(err);
    }
  }
}

export function wsURL(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/ws`;
}
