import { Calcron } from "@bzrlab/calcron";

type Member = { guildId: string; userId: string };
/** One membership: a rejoin starts a new verification with its own correlation key. */
export type Join = Member & { joinedAt: string };
export type Verdict = Join & { approved: boolean; by: string; reason: string };

export type Events = {
  "reminder.due": { reminderId: string; userId: string; channelId: string; text: string };
  "ticket.idle": { threadId: string };
  "ticket.close": { threadId: string };
  "temprole.expire": Member & { roleId: string };
  "staff.alert": Member & { channelId: string; reason: string };
  "giveaway.end": { channelId: string; messageId: string; prize: string; winners: number };
  "verify.prompt": Join;
  "verify.timeout": Join;
  "verify.grant": Verdict;
  "verify.reject": Verdict;
  "member.verified": Verdict;
  "standup.open": Standup;
  "standup.close": Standup;
};
/** `name` is set for ad-hoc standups so their close only archives their own thread. */
export type Standup = { channelId: string; name?: string };
export type Workflows = {
  "member-verification": Join;
  "daily-standup": Standup;
};

export function env(name: string, fallback?: string): string {
  const value = process.env[name] || fallback;
  if (value === undefined) throw new Error(`${name} is required`);
  return value;
}

const url = env("CALCRON_URL", "wss://calcron.bzr.lt/ws");
export const calcron = new Calcron<Events, Workflows>(url, process.env.CALCRON_TOKEN ?? "");

export const verifyKey = (j: Join) => `verify:${j.guildId}:${j.userId}:${j.joinedAt}`;

const DURATION = /^(\d+(\.\d+)?(ms|s|m|h))+$/;
export const isDuration = (value: string) => DURATION.test(value);

/** Accepts a Go duration (`10m`, `1h30m`) or an absolute date; returns the matching Calcron deadline. */
export function parseWhen(input: string, now = Date.now()): { after: string } | { at: string } {
  const value = input.trim().replace(/\s+/g, "");
  if (isDuration(value)) return { after: value };
  const unix = /^\d{10,}$/.test(value);
  const at = new Date(unix ? Number(value) * 1000 : /(Z|[+-]\d\d:?\d\d)$/i.test(value) ? input.trim() : NaN);
  if (Number.isNaN(at.getTime())) throw new Error("Use a duration like `10m`, `2h30m`, a unix timestamp, or a date with a timezone like `2026-10-01T09:00+06:00`.");
  if (at.getTime() <= now) throw new Error("That time is in the past.");
  return { at: at.toISOString().replace(/\.\d{3}Z$/, "Z") };
}

/** Discord relative timestamp for a Calcron `runAt`. */
export const relative = (runAt: string | Date) => `<t:${Math.floor(new Date(runAt).getTime() / 1000)}:R>`;

type AdminReply = { id?: string; ok?: boolean; data?: unknown; error?: string };

/** Short-lived administrator connection. Admin operations are not part of the application SDK. */
export class Admin {
  private ws: WebSocket;
  private n = 0;
  private waiting = new Map<string, (reply: AdminReply) => void>();
  private constructor(ws: WebSocket) {
    this.ws = ws;
    ws.onmessage = e => { const reply: AdminReply = JSON.parse(String(e.data)); this.waiting.get(reply.id!)?.(reply); this.waiting.delete(reply.id!); };
    ws.onclose = () => { for (const done of this.waiting.values()) done({ error: "Calcron disconnected" }); this.waiting.clear(); };
  }
  static async connect(to = url, token = env("CALCRON_ADMIN_TOKEN")) {
    const ws = new WebSocket(to);
    await new Promise<void>((ok, bad) => { ws.onopen = () => ok(); ws.onerror = () => bad(new Error("Calcron connection failed")); });
    const admin = new Admin(ws);
    const auth = await admin.call<{ admin: boolean }>("auth", { token }).catch(error => { ws.close(); throw error; });
    if (!auth.admin) { ws.close(); throw new Error("CALCRON_ADMIN_TOKEN is not an administrator token"); }
    return admin;
  }
  call<T>(op: string, body: Record<string, unknown> = {}) {
    const id = String(++this.n);
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => { this.waiting.delete(id); reject(new Error(`${op} timed out`)); }, 10_000);
      this.waiting.set(id, reply => { clearTimeout(timer); reply.ok ? resolve(reply.data as T) : reject(new Error(reply.error ?? `${op} failed`)); });
      this.ws.send(JSON.stringify({ id, op, ...body }));
    });
  }
  close() { this.ws.close(); }
}

export async function withAdmin<T>(fn: (admin: Admin) => Promise<T>) {
  const admin = await Admin.connect();
  try { return await fn(admin); } finally { admin.close(); }
}
