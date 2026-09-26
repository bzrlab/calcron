import { useState } from "react";
import { useAdmin } from "../lib/admin";

type Tab = "workflow" | "calendar" | "start";

const SAMPLE_WORKFLOW = {
  initial: "wait",
  states: {
    wait: { type: "wait_signal", event: "payment.confirmed", correlationKey: "order-1", next: "branch" },
    branch: { type: "branch", when: "state.signal.ok == true", true: "emit", false: "end" },
    emit: { type: "emit", target: "REPLACE_WITH_APPLICATION_ID", event: "payment.close", next: "end" },
    end: { type: "end" },
  },
};

const SAMPLE_CALENDAR = {
  timezone: "Asia/Dhaka",
  weekdays: [0, 1, 2, 3, 4, 5, 6],
  overrides: {},
};

export function Publish() {
  const { send } = useAdmin();
  const [tab, setTab] = useState<Tab>("workflow");
  const [appId, setAppId] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null);

  // workflow
  const [wfName, setWfName] = useState("");
  const [wfDef, setWfDef] = useState(JSON.stringify(SAMPLE_WORKFLOW, null, 2));

  // calendar
  const [calName, setCalName] = useState("");
  const [calDef, setCalDef] = useState(JSON.stringify(SAMPLE_CALENDAR, null, 2));

  // start schedule
  const [ssName, setSsName] = useState("");
  const [ssWorkflow, setSsWorkflow] = useState("");
  const [ssCalendar, setSsCalendar] = useState("");
  const [ssLocalTime, setSsLocalTime] = useState("09:00");
  const [ssPolicy, setSsPolicy] = useState("skip");

  async function run(body: Record<string, unknown> & { op: string }) {
    setBusy(true);
    setResult(null);
    const r = await send(body);
    setBusy(false);
    setResult({ ok: r.ok, text: r.ok ? JSON.stringify(r.data) : r.error ?? "failed" });
  }

  function parse(raw: string): unknown | undefined {
    try {
      return JSON.parse(raw);
    } catch {
      setResult({ ok: false, text: "Invalid JSON" });
      return undefined;
    }
  }

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-xl font-semibold">Publish</h2>
        <p className="text-sm text-base-content/60">
          Admin-only configuration. Definitions publish as immutable versions.
        </p>
      </header>

      <div role="tablist" className="tabs tabs-boxed w-fit">
        {(["workflow", "calendar", "start"] as Tab[]).map((t) => (
          <button
            key={t}
            role="tab"
            className={`tab ${tab === t ? "tab-active" : ""}`}
            onClick={() => { setTab(t); setResult(null); }}
          >
            {t === "workflow" ? "Workflow definition" : t === "calendar" ? "Business calendar" : "Start schedule"}
          </button>
        ))}
      </div>

      <label className="form-control w-full max-w-md">
        <span className="label-text mb-1">Application ID</span>
        <input
          className="input input-bordered font-mono"
          value={appId}
          onChange={(e) => setAppId(e.target.value)}
          placeholder="application id from the Applications view"
        />
      </label>

      {tab === "workflow" && (
        <div className="space-y-4">
          <label className="form-control w-full max-w-md">
            <span className="label-text mb-1">Workflow name</span>
            <input className="input input-bordered font-mono" value={wfName} onChange={(e) => setWfName(e.target.value)} />
          </label>
          <label className="form-control w-full">
            <span className="label-text mb-1">Definition (JSON)</span>
            <textarea
              className="textarea textarea-bordered h-72 font-mono text-xs"
              value={wfDef}
              onChange={(e) => setWfDef(e.target.value)}
              spellCheck={false}
            />
          </label>
          <button
            className="btn btn-primary"
            disabled={busy || !appId || !wfName}
            onClick={() => {
              const data = parse(wfDef);
              if (data === undefined) return;
              void run({ op: "workflow.publish", applicationId: appId, name: wfName, data });
            }}
          >
            {busy && <span className="loading loading-spinner loading-sm" />}Publish version
          </button>
        </div>
      )}

      {tab === "calendar" && (
        <div className="space-y-4">
          <label className="form-control w-full max-w-md">
            <span className="label-text mb-1">Calendar name</span>
            <input className="input input-bordered font-mono" value={calName} onChange={(e) => setCalName(e.target.value)} />
          </label>
          <label className="form-control w-full">
            <span className="label-text mb-1">Definition (JSON) — timezone, weekdays, overrides</span>
            <textarea
              className="textarea textarea-bordered h-56 font-mono text-xs"
              value={calDef}
              onChange={(e) => setCalDef(e.target.value)}
              spellCheck={false}
            />
          </label>
          <button
            className="btn btn-primary"
            disabled={busy || !appId || !calName}
            onClick={() => {
              const data = parse(calDef);
              if (data === undefined) return;
              void run({ op: "calendar.set", applicationId: appId, name: calName, data });
            }}
          >
            {busy && <span className="loading loading-spinner loading-sm" />}Set calendar
          </button>
        </div>
      )}

      {tab === "start" && (
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2 max-w-2xl">
            <label className="form-control w-full">
              <span className="label-text mb-1">Name</span>
              <input className="input input-bordered font-mono" value={ssName} onChange={(e) => setSsName(e.target.value)} />
            </label>
            <label className="form-control w-full">
              <span className="label-text mb-1">Workflow</span>
              <input className="input input-bordered font-mono" value={ssWorkflow} onChange={(e) => setSsWorkflow(e.target.value)} />
            </label>
            <label className="form-control w-full">
              <span className="label-text mb-1">Calendar</span>
              <input className="input input-bordered font-mono" value={ssCalendar} onChange={(e) => setSsCalendar(e.target.value)} />
            </label>
            <label className="form-control w-full">
              <span className="label-text mb-1">Local time (HH:MM)</span>
              <input className="input input-bordered font-mono" value={ssLocalTime} onChange={(e) => setSsLocalTime(e.target.value)} />
            </label>
            <label className="form-control w-full">
              <span className="label-text mb-1">Missed occurrence policy</span>
              <select className="select select-bordered" value={ssPolicy} onChange={(e) => setSsPolicy(e.target.value)}>
                <option value="skip">skip</option>
                <option value="run_once_late">run_once_late</option>
                <option value="catch_up">catch_up</option>
              </select>
            </label>
          </div>
          <button
            className="btn btn-primary"
            disabled={busy || !appId || !ssName || !ssWorkflow || !ssCalendar}
            onClick={() =>
              void run({
                op: "start-schedule.set",
                applicationId: appId,
                name: ssName,
                workflow: ssWorkflow,
                calendar: ssCalendar,
                localTime: ssLocalTime,
                missedPolicy: ssPolicy,
              })
            }
          >
            {busy && <span className="loading loading-spinner loading-sm" />}Set start schedule
          </button>
        </div>
      )}

      {result && (
        <div className={`alert ${result.ok ? "alert-success" : "alert-error"} max-w-2xl`}>
          <span className="font-mono text-xs break-all">{result.text}</span>
        </div>
      )}
    </div>
  );
}
