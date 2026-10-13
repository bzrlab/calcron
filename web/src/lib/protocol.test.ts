// A request that gets no reply must drop the socket. A half-open connection
// never closes on its own, and a late close must not fail the next session.
import assert from "node:assert/strict";
import { AdminClient } from "./protocol.ts";

class FakeSocket {
  static OPEN = 1;
  static sockets: FakeSocket[] = [];
  readyState = FakeSocket.OPEN;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  closed = false;

  constructor(_url: string) {
    FakeSocket.sockets.push(this);
    queueMicrotask(() => this.onopen?.());
  }

  send(raw: string) {
    const frame = JSON.parse(raw) as { id?: string; op?: string };
    if (frame.op === "auth") {
      this.onmessage?.({ data: JSON.stringify({ id: frame.id, ok: true, data: { application: "admin", admin: true } }) });
    }
  }

  close() {
    this.closed = true;
    this.readyState = 3;
    this.onclose?.();
  }
}

FakeSocket.sockets = [];
globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket;

const client = new AdminClient("ws://calcron.test/ws", "admin");
let state = "";
client.setHandlers({ onState: (s) => { state = s; } });
await client.connect();
const socket = FakeSocket.sockets[0]!;
const hung = client.request({ op: "dashboard.list", name: "apps" }, 5_000);
const timed = client.request({ op: "dashboard.stats" }, 30);
await assert.rejects(timed, /timeout: dashboard.stats/);
await assert.rejects(hung, /Calcron disconnected/);
assert.equal(socket.closed, true);
assert.equal(socket.onclose, null);
assert.equal(state, "closed");

console.log("protocol verification passed");
