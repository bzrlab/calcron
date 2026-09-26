import assert from "node:assert/strict";
import test from "node:test";
import { WebSocketServer } from "ws";
import { Calcron } from "./index.ts";

test("reconnects after server closes connection", async () => {
  const server = new WebSocketServer({ port: 0 });
  await new Promise<void>(ok => server.once("listening", ok));
  let count = 0;
  const again = new Promise<void>(ok => server.on("connection", socket => {
    socket.once("message", raw => {
      const { id } = JSON.parse(String(raw));
      socket.send(JSON.stringify({ id, ok: true }));
      if (++count === 1) setTimeout(() => socket.close(), 100);
      else ok();
    });
  }));
  const port = (server.address() as { port: number }).port;
  const client = new Calcron(`ws://127.0.0.1:${port}`, "test");
  await client.connect();
  let timer: ReturnType<typeof setTimeout>;
  await Promise.race([again, new Promise((_, fail) => { timer = setTimeout(() => fail(new Error("no reconnect")), 3_000); })]);
  clearTimeout(timer!);
  assert.equal(count, 2);
  client.close();
  await new Promise<void>(ok => server.close(() => ok()));
});
