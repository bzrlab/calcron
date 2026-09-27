// Calcron admin protocol client. Mirrors internal/server frame + reply shapes
// (see .cursor/rules/protocol-and-sdk.mdc). Admin connects with the admin token,
// sends one frame per op, correlates replies by id.

export type Reply = {
  id?: string;
  ok: boolean;
  data?: unknown;
  error?: string;
};

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
  request(
    body: Record<string, unknown> & { op: string },
    timeoutMs = 10_000,
  ): Promise<Reply> {
    const id = String(++this.n);
    return new Promise<Reply>((resolve, reject) => {
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
          resolve(r);
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
