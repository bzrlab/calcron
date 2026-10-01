import { useState, type ReactNode } from "react";

type Language = "node" | "go";

const links = [
  ["Quickstart", "quickstart"],
  ["Connect", "connect"],
  ["Schedule work", "scheduling"],
  ["Recurring workflows", "recurring"],
  ["Handle deliveries", "deliveries"],
  ["Operations", "operations"],
  ["Reliability", "reliability"],
] as const;

const examples: Record<Language, string> = {
  node: `import { Calcron } from "@bzrlab/calcron";

type Events = {
  "invoice.due": { invoiceId: string; amount: number };
};

const cron = new Calcron<Events>(
  process.env.CALCRON_URL!,
  process.env.CALCRON_TOKEN!,
);

cron.on("invoice.due", async delivery => {
  await saveReceiptOnce(delivery.id, delivery.data);
  await delivery.ack();
});

await cron.connect();
const schedule = await cron.set({
  key: "invoice:42",
  event: "invoice.due",
  after: "24h",
  data: { invoiceId: "42", amount: 1250 },
  idempotencyKey: "invoice:42:due:v1",
});
console.log(schedule.runAt);`,
  go: `package main

import (
  "context"
  "log"
  "os"

  cron "github.com/bzrlab/calcron/sdk/go"
)

func main() {
ctx := context.Background()
client, err := cron.Connect(ctx,
  os.Getenv("CALCRON_URL"), os.Getenv("CALCRON_TOKEN"))
if err != nil { log.Fatal(err) }
defer client.Close()

type InvoiceDue struct {
  InvoiceID string  \`json:"invoiceId"\`
  Amount    float64 \`json:"amount"\`
}
event := cron.EventOf[InvoiceDue]("invoice.due")
cron.OnTyped(client, event, func(delivery cron.TypedDelivery[InvoiceDue]) {
  if err := saveReceiptOnce(delivery.ID, delivery.Data); err != nil { return }
  _, _ = delivery.Ack(context.Background())
})

result, err := cron.SetTyped(ctx, client, cron.TypedSchedule[InvoiceDue]{
  Key: "invoice:42", Event: event, After: "24h",
  Data: InvoiceDue{InvoiceID: "42", Amount: 1250},
  IdempotencyKey: "invoice:42:due:v1",
})
if err != nil { log.Fatal(err) }
log.Println(result.RunAt)
}`,
};

function CodeBlock({ language, code, onLanguage }: { language: Language; code: string; onLanguage: (value: Language) => void }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    await navigator.clipboard.writeText(code);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  }

  return (
    <div className="overflow-hidden rounded-2xl border border-white/10 bg-[#0d111b] shadow-2xl shadow-black/20">
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
        <div className="flex gap-1" aria-label="Example language">
          {(["node", "go"] as const).map((item) => (
            <button key={item} className={`btn btn-xs ${language === item ? "btn-primary" : "btn-ghost text-base-content/60"}`} onClick={() => onLanguage(item)} aria-pressed={language === item}>
              {item === "node" ? "Node.js" : "Go"}
            </button>
          ))}
        </div>
        <button className="btn btn-ghost btn-xs text-base-content/60" onClick={() => void copy()} aria-label="Copy code example">
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="m-0 overflow-x-auto p-5 text-xs leading-6 text-slate-200 sm:text-sm"><code>{code}</code></pre>
    </div>
  );
}

function SectionTitle({ eyebrow, title, children }: { eyebrow: string; title: string; children: ReactNode }) {
  return <div className="mb-6 max-w-2xl"><p className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-primary">{eyebrow}</p><h2 className="mb-3 text-2xl font-semibold tracking-tight sm:text-3xl">{title}</h2><p className="text-sm leading-6 text-base-content/65 sm:text-base">{children}</p></div>;
}

export function Docs() {
  const [language, setLanguage] = useState<Language>("node");

  return (
    <div className="min-h-screen bg-base-200 text-base-content">
      <header className="sticky top-0 z-20 border-b border-base-300/80 bg-base-100/90 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-[1440px] items-center justify-between px-5 sm:px-8">
          <a href="/" className="flex items-center gap-3 font-semibold">
            <span className="grid h-8 w-8 place-items-center rounded-lg bg-primary text-sm text-primary-content">C</span>
            <span>Calcron <span className="font-normal text-base-content/45">/ Docs</span></span>
          </a>
          <nav aria-label="Top navigation" className="flex items-center gap-2 sm:gap-4">
            <a className="hidden text-sm text-base-content/65 hover:text-base-content sm:inline" href="https://github.com/bzrlab/calcron/blob/main/docs/sdk.md">SDK reference</a>
            <a className="btn btn-sm btn-primary" href="/">Open dashboard <span aria-hidden="true">↗</span></a>
          </nav>
        </div>
      </header>

      <div className="mx-auto grid max-w-[1440px] grid-cols-1 gap-10 px-5 sm:px-8 lg:grid-cols-[220px_minmax(0,820px)] lg:gap-14">
        <nav aria-label="Documentation sections" className="sticky top-16 z-10 -mx-5 flex gap-1 overflow-x-auto border-b border-base-300 bg-base-200 px-4 py-2 lg:hidden">
          {links.map(([label, id]) => <a key={id} href={`#${id}`} className="shrink-0 rounded-lg px-3 py-2 text-xs text-base-content/65 hover:bg-base-100 hover:text-base-content">{label}</a>)}
        </nav>
        <aside className="hidden lg:block">
          <nav aria-label="Documentation sections" className="sticky top-24 space-y-1 py-10">
            <p className="mb-3 px-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-base-content/40">Get started</p>
            {links.map(([label, id], index) => (
              <a key={id} href={`#${id}`} className="flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-base-content/65 transition hover:bg-base-100 hover:text-base-content">
                <span className="font-mono text-[11px] text-base-content/35">0{index + 1}</span>{label}
              </a>
            ))}
            <div className="mt-8 rounded-xl border border-base-300 bg-base-100 p-4">
              <p className="text-sm font-medium">Need the full API?</p>
              <a className="mt-2 inline-block text-xs text-primary hover:underline" href="https://github.com/bzrlab/calcron/blob/main/docs/sdk.md">Read SDK reference ↗</a>
            </div>
          </nav>
        </aside>

        <main className="min-w-0 pb-24">
          <section className="border-b border-base-300 py-12 sm:py-16">
            <div className="mb-5 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
              <span className="h-1.5 w-1.5 rounded-full bg-primary" /> Durable scheduling for your apps
            </div>
            <h1 className="max-w-3xl text-4xl font-semibold tracking-tight sm:text-5xl">Ship work on time.<br /><span className="text-base-content/45">Keep every delivery.</span></h1>
            <p className="mt-5 max-w-2xl text-base leading-7 text-base-content/65">Connect an application over WebSocket, schedule an event, then acknowledge delivery after your app saves its work. This guide gets a Node or Go client running.</p>
            <div className="mt-7 flex flex-wrap gap-3">
              <a className="btn btn-primary" href="#quickstart">Start quickstart <span aria-hidden="true">↓</span></a>
              <a className="btn btn-outline" href="https://github.com/bzrlab/calcron/blob/main/docs/sdk.md">Browse full SDK guide</a>
            </div>
            <div className="mt-10 grid gap-3 sm:grid-cols-3">
              {[["Transport", "WebSocket"], ["Delivery", "At least once"], ["Schedule time", "UTC or calendar local time"]].map(([label, value]) => (
                <div key={label} className="rounded-xl border border-base-300 bg-base-100/70 p-4"><p className="text-xs text-base-content/45">{label}</p><p className="mt-1 text-sm font-medium">{value}</p></div>
              ))}
            </div>
          </section>

          <section id="quickstart" className="scroll-mt-24 border-b border-base-300 py-12 sm:py-14">
            <SectionTitle eyebrow="01 / Quickstart" title="Connect and schedule your first event">Install an SDK, register a delivery handler, connect, and create a one-time schedule.</SectionTitle>
            <div className="mb-5 rounded-xl border border-base-300 bg-base-100 p-4 sm:flex sm:items-center sm:justify-between">
              <div><p className="text-sm font-medium">Install the client</p><p className="mt-1 text-xs text-base-content/55">Node.js 22+ or Go 1.25+</p></div>
              <code className="mt-3 block rounded-lg bg-base-300/60 px-3 py-2 text-xs sm:mt-0">{language === "node" ? "npm install @bzrlab/calcron" : "go get github.com/bzrlab/calcron/sdk/go"}</code>
            </div>
            <CodeBlock language={language} code={examples[language]} onLanguage={setLanguage} />
            <p className="mt-4 text-xs leading-5 text-base-content/55">Replace <code>saveReceiptOnce</code> with an idempotent write in your app. Keep the token in environment configuration.</p>
          </section>

          <section id="connect" className="scroll-mt-24 border-b border-base-300 py-12 sm:py-14">
            <SectionTitle eyebrow="02 / Connection" title="One connection for commands and events">The SDK authenticates with an application token in the first WebSocket frame. Keep credentials out of URLs and source control.</SectionTitle>
            <div className="grid gap-4 sm:grid-cols-2">
              <article className="rounded-xl border border-base-300 bg-base-100 p-5"><p className="text-xs font-semibold uppercase tracking-wider text-base-content/40">Server URL</p><p className="mt-3 font-mono text-sm">wss://your-host/ws</p><p className="mt-2 text-xs leading-5 text-base-content/55">Use <code>ws://localhost:8080/ws</code> for local development.</p></article>
              <article className="rounded-xl border border-base-300 bg-base-100 p-5"><p className="text-xs font-semibold uppercase tracking-wider text-base-content/40">Environment</p><p className="mt-3 font-mono text-sm">CALCRON_URL</p><p className="mt-2 text-xs leading-5 text-base-content/55">Your WebSocket endpoint. Provide the issued app token as <code>CALCRON_TOKEN</code>.</p></article>
            </div>
            <div className="mt-4 rounded-xl border border-warning/20 bg-warning/5 p-4 text-sm leading-6"><span className="font-medium">Admin and app tokens have different access.</span> App tokens schedule and manage work for one application. Publishing workflows, editing calendars, and delivery recovery require an admin token.</div>
          </section>

          <section id="scheduling" className="scroll-mt-24 border-b border-base-300 py-12 sm:py-14">
            <SectionTitle eyebrow="03 / Scheduling" title="Choose a one-time deadline">Use a duration for work relative to now, or an RFC 3339 timestamp for a fixed instant. The schedule key identifies the application-owned schedule.</SectionTitle>
            <div className="overflow-x-auto rounded-xl border border-base-300 bg-base-100">
              <table className="table table-sm"><thead><tr><th>Intent</th><th>Example</th><th>Behavior</th></tr></thead><tbody>
                <tr><td>Relative</td><td><code>after: "30m"</code></td><td>Due 30 minutes after creation.</td></tr>
                <tr><td>Fixed time</td><td><code>at: "2026-10-02T09:00:00Z"</code></td><td>Due at the UTC instant given.</td></tr>
                <tr><td>Throttle</td><td><code>cooldown: "5m"</code></td><td>Send immediately once, then suppress repeats for five minutes.</td></tr>
              </tbody></table>
            </div>
            <p className="mt-4 text-sm leading-6 text-base-content/65">A schedule with the same key replaces the existing one for that application. Use a stable <code>idempotencyKey</code> when retrying the same request after a timeout or disconnect. Use a new key for a new intent.</p>
          </section>

          <section id="recurring" className="scroll-mt-24 border-b border-base-300 py-12 sm:py-14">
            <SectionTitle eyebrow="04 / Recurrence" title="Recurring starts use calendars">Calcron does not parse cron strings such as <code>* * * * *</code>. Set up a named calendar, then create a start schedule with a local time and missed-run policy.</SectionTitle>
            <div className="rounded-2xl border border-base-300 bg-base-100 p-5 sm:p-6">
              <div className="grid gap-4 sm:grid-cols-[1fr_auto_1fr_auto_1fr] sm:items-center">
                {["Calendar\nWeekdays, timezone, overrides", "Local time\n09:00", "Start schedule\nWorkflow + missed policy"].map((item, index) => (
                  <div key={item} className="contents">
                    <div className="rounded-xl border border-base-300 bg-base-200 p-4"><p className="whitespace-pre-line text-sm font-medium">{item.split("\n")[0]}</p><p className="mt-1 text-xs text-base-content/55">{item.split("\n")[1]}</p></div>
                    {index < 2 && <span className="hidden text-center text-base-content/35 sm:block" aria-hidden="true">→</span>}
                  </div>
                ))}
              </div>
              <div className="mt-5 border-t border-base-300 pt-4">
                <p className="text-sm font-medium">When a run is missed</p>
                <div className="mt-3 grid gap-3 sm:grid-cols-3">
                  {[["skip", "Move to the next calendar time."], ["run_once_late", "Start once when the service is back."], ["catch_up", "Start missed occurrences in sequence."]].map(([policy, description]) => <div key={policy}><code className="text-xs text-primary">{policy}</code><p className="mt-1 text-xs leading-5 text-base-content/55">{description}</p></div>)}
                </div>
              </div>
            </div>
          </section>

          <section id="deliveries" className="scroll-mt-24 border-b border-base-300 py-12 sm:py-14">
            <SectionTitle eyebrow="05 / Delivery" title="Save first, acknowledge second">Calcron delivers at least once. If a connection drops before acknowledgement, the same delivery ID may arrive again.</SectionTitle>
            <ol className="space-y-3">
              {["Receive the event and its delivery ID.", "Write the business change and receipt in one durable, idempotent operation.", "Acknowledge only after that write succeeds."].map((step, index) => <li key={step} className="flex gap-4 rounded-xl border border-base-300 bg-base-100 p-4"><span className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-primary/15 font-mono text-xs text-primary">{index + 1}</span><span className="pt-1 text-sm leading-5">{step}</span></li>)}
            </ol>
            <p className="mt-4 text-sm leading-6 text-base-content/65">Acknowledgement means your app durably received the event. It does not confirm a payment, shipment, or other external outcome. Use <code>signal</code> with the workflow's event and correlation key to report that outcome.</p>
          </section>

          <section id="operations" className="scroll-mt-24 border-b border-base-300 py-12 sm:py-14">
            <SectionTitle eyebrow="06 / Reference" title="Application operations">Every state-changing operation requires an application-supplied idempotency key.</SectionTitle>
            <div className="overflow-x-auto rounded-xl border border-base-300 bg-base-100">
              <table className="table table-sm"><thead><tr><th>Operation</th><th>Required fields</th><th>Returns</th></tr></thead><tbody>
                <tr><td><code>schedule.set</code></td><td>key, event, after or at, idempotencyKey</td><td>scheduleId, runAt</td></tr>
                <tr><td><code>schedule.cancel</code></td><td>key, idempotencyKey</td><td>cancelled</td></tr>
                <tr><td><code>schedule.extend</code></td><td>key, by, idempotencyKey</td><td>runAt</td></tr>
                <tr><td><code>schedule.throttle</code></td><td>key, event, cooldown, idempotencyKey</td><td>triggered</td></tr>
                <tr><td><code>workflow.start</code></td><td>name, idempotencyKey, optional data</td><td>instanceId</td></tr>
                <tr><td><code>signal</code></td><td>event, correlationKey, idempotencyKey</td><td>matched</td></tr>
                <tr><td><code>delivery.ack</code></td><td>deliveryId, idempotencyKey</td><td>acked</td></tr>
              </tbody></table>
            </div>
            <p className="mt-4 text-xs leading-5 text-base-content/55">Durations use Go duration syntax, for example <code>30s</code>, <code>5m</code>, or <code>24h</code>. See the full reference for payload shapes and validation errors.</p>
          </section>

          <section id="reliability" className="scroll-mt-24 py-12 sm:py-14">
            <SectionTitle eyebrow="07 / Reliability" title="Plan for reconnects and retries">SDKs reconnect after transport loss. A request that was in flight during a disconnect fails locally; retry that same intent with the same idempotency key.</SectionTitle>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="rounded-xl border border-base-300 bg-base-100 p-5"><p className="text-sm font-medium">Make writes safe to repeat</p><p className="mt-2 text-xs leading-5 text-base-content/55">Deduplicate delivery side effects by delivery ID or business key before acknowledging.</p></div>
              <div className="rounded-xl border border-base-300 bg-base-100 p-5"><p className="text-sm font-medium">Separate receipt from completion</p><p className="mt-2 text-xs leading-5 text-base-content/55">Acknowledge after durable handling. Signal a workflow later when the external work finishes.</p></div>
            </div>
            <div className="mt-8 flex flex-col justify-between gap-4 rounded-2xl border border-primary/20 bg-primary/5 p-5 sm:flex-row sm:items-center sm:p-6">
              <div><p className="font-medium">Ready for the full reference?</p><p className="mt-1 text-sm text-base-content/60">Typed APIs, every SDK method, complete examples, and workflow notes.</p></div>
              <a className="btn btn-primary btn-sm" href="https://github.com/bzrlab/calcron/blob/main/docs/sdk.md">Open SDK guide ↗</a>
            </div>
          </section>
        </main>
      </div>
    </div>
  );
}
