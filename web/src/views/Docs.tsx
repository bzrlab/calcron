const operations = [
  ["schedule.set", "Create or replace a schedule. Include key, event, and exactly one of after or at."],
  ["schedule.cancel", "Cancel a schedule by key. Cancelling an absent schedule is safe."],
  ["schedule.extend", "Add a duration to the current deadline, or from now if it is already due. Include by."],
  ["schedule.throttle", "Create one immediate delivery for a key, then suppress triggers during cooldown."],
  ["workflow.start", "Start the newest published workflow version. Include name and optional data."],
  ["signal", "Signal waiting workflow instances. Include event and correlationKey."],
  ["delivery.ack", "Acknowledge a delivery using deliveryId."],
];

export function Docs() {
  return (
    <main className="min-h-screen bg-base-200 text-base-content">
      <header className="border-b border-base-300 bg-base-100">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-5">
          <a href="/" className="font-semibold">Calcron</a>
          <a href="/" className="btn btn-sm btn-outline">Admin sign in</a>
        </div>
      </header>
      <article className="prose prose-sm mx-auto max-w-5xl px-6 py-10 lg:prose-base">
        <p className="text-sm font-semibold uppercase tracking-widest text-primary">Application protocol</p>
        <h1>WebSocket API</h1>
        <p>Calcron exposes application commands and durable event delivery over one authenticated WebSocket connection.</p>

        <h2>Connect</h2>
        <p>Connect to <code>wss://your-calcron-host/ws</code> in production (or <code>ws://localhost:8080/ws</code> locally). Send an authentication frame first. Keep the token out of URLs and logs.</p>
        <pre><code>{`{"id":"1","op":"auth","token":"YOUR_APPLICATION_TOKEN"}`}</code></pre>
        <p>A successful reply includes the authenticated application. A rejected token returns <code>unauthorized</code> and closes the connection.</p>

        <h2>Request and reply frames</h2>
        <p>Send JSON text frames. Requests use a caller-generated <code>id</code>; replies echo it. Successful replies include <code>ok</code> and, when applicable, <code>data</code>. Errors include <code>error</code>.</p>
        <pre><code>{`{"id":"2","op":"schedule.set","key":"invoice:42","event":"invoice.due","after":"24h","data":{"invoiceId":"42"},"idempotencyKey":"invoice:42:due:v1"}
{"id":"2","ok":true,"data":{"scheduleId":"…","runAt":"2026-10-02T12:00:00Z"}}`}</code></pre>
        <p>Every state-changing application request requires an <code>idempotencyKey</code>. Reuse the same key only when retrying that same intended change after an uncertain result.</p>

        <h2>Application operations</h2>
        <div className="not-prose overflow-x-auto rounded-box border border-base-300 bg-base-100">
          <table className="table table-zebra">
            <thead><tr><th>Operation</th><th>Purpose</th></tr></thead>
            <tbody>{operations.map(([op, description]) => <tr key={op}><td><code>{op}</code></td><td>{description}</td></tr>)}</tbody>
          </table>
        </div>
        <p>Durations use Go duration syntax such as <code>30s</code>, <code>5m</code>, and <code>24h</code>. Absolute deadlines use RFC 3339 timestamps.</p>

        <h2>Deliveries</h2>
        <p>Calcron sends due events as unsolicited frames. They include <code>op: "delivery"</code>, <code>deliveryId</code>, <code>event</code>, and <code>data</code>.</p>
        <pre><code>{`{"op":"delivery","deliveryId":"…","event":"invoice.due","data":{"invoiceId":"42"}}`}</code></pre>
        <p>Delivery is at least once, so the same ID can arrive again after a disconnect or missing acknowledgement. Make processing idempotent, then acknowledge:</p>
        <pre><code>{`{"id":"3","op":"delivery.ack","deliveryId":"…","idempotencyKey":"ack:…"}`}</code></pre>
        <p>An acknowledgement confirms durable receipt; it does not mean the business action has completed. Use <code>signal</code> to report an external outcome to a waiting workflow.</p>

        <h2>SDKs and guides</h2>
        <ul>
          <li><a href="https://github.com/bzrlab/calcron/blob/main/sdk/node/README.md">Node / TypeScript SDK guide</a></li>
          <li><a href="https://github.com/bzrlab/calcron/blob/main/sdk/go/README.md">Go SDK guide</a></li>
        </ul>
        <p>Application tokens can perform application operations only. Workflow publishing, calendar management, and recovery actions require an administrator.</p>
      </article>
    </main>
  );
}
