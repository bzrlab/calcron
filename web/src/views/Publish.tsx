import { useEffect, useState } from "react";
import { useAdmin } from "../lib/admin";
import { useList } from "../lib/useList";
import { useShell } from "../lib/shell";
import { jsonError } from "../lib/dashboard";
import { Chips } from "../components/ui";

type Tab = "workflow" | "start";
type Version = { application_id: string; name: string; version: number };
type Calendar = { application_id: string; name: string };

const SAMPLE_WORKFLOW = {
  initial: "wait",
  states: {
    wait: { type: "wait_signal", event: "payment.confirmed", correlationKey: "order-1", next: "branch" },
    branch: { type: "branch", when: "state.signal.ok == true", true: "emit", false: "end" },
    emit: { type: "emit", target: "REPLACE_WITH_APPLICATION_ID", event: "payment.close", next: "end" },
    end: { type: "end" },
  },
};

export function Publish() {
  const { apps } = useShell();
  const [tab, setTab] = useState<Tab>("workflow");
  const [appId, setAppId] = useState("");

  useEffect(() => {
    if (!appId && apps[0]) setAppId(apps[0].id);
  }, [apps, appId]);

  return (
    <div className="max-w-3xl space-y-6">
      <header>
        <h2 className="text-xl font-semibold">Publish</h2>
        <p className="text-sm text-base-content/60">
          Admin-only configuration. Workflow definitions publish as immutable versions; paste a definition, it is not edited here.
          Business calendars are edited in Calendars.
        </p>
      </header>

      <div className="flex flex-wrap items-end gap-4">
        <Chips<Tab>
          value={tab}
          onChange={setTab}
          options={[{ value: "workflow", label: "Workflow definition" }, { value: "start", label: "Start schedule" }]}
        />
        <label className="form-control w-64">
          <span className="label-text mb-1">Application</span>
          <select className="select select-bordered select-sm" value={appId} onChange={(e) => setAppId(e.target.value)}>
            {apps.length === 0 && <option value="">Register an application first</option>}
            {apps.map((a) => <option key={a.id} value={a.id}>{a.namespace}</option>)}
          </select>
        </label>
      </div>

      {tab === "workflow" ? <WorkflowForm appId={appId} /> : <StartScheduleForm key={appId} appId={appId} />}
    </div>
  );
}

function WorkflowForm({ appId }: { appId: string }) {
  const { send } = useAdmin();
  const { notify, appName, go, bump } = useShell();
  const [name, setName] = useState("");
  const [definition, setDefinition] = useState("");
  const [serverError, setServerError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const parseError = definition.trim() ? jsonError(definition) : null;
  const error = parseError?.message ?? serverError;

  async function publish() {
    setBusy(true);
    setServerError(null);
    try {
      const r = await send({ op: "workflow.publish", applicationId: appId, name, data: JSON.parse(definition) });
      if (!r.ok) {
        setServerError(r.error ?? "workflow.publish failed");
        return;
      }
      const version = (r.data as { version: number }).version;
      notify(`Published ${name} v${version} for ${appName(appId)}`);
      bump();
      go("workflows");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <label className="form-control w-full max-w-md">
        <span className="label-text mb-1">Workflow name</span>
        <input className="input input-bordered input-sm font-mono" value={name} onChange={(e) => setName(e.target.value)} />
      </label>
      <div className="flex items-center justify-between">
        <label htmlFor="definition" className="label-text">Definition (JSON)</label>
        {!definition && (
          <button type="button" className="btn btn-ghost btn-xs" onClick={() => setDefinition(JSON.stringify(SAMPLE_WORKFLOW, null, 2))}>
            Use sample
          </button>
        )}
      </div>
      {/* ponytail: plain textarea. Highlighting/autocomplete make it an editor, which ADR 0019 rules out for V1. */}
      <textarea
        id="definition"
        className={`textarea textarea-bordered h-72 w-full font-mono text-xs ${error ? "textarea-error" : ""}`}
        value={definition}
        onChange={(e) => { setDefinition(e.target.value); setServerError(null); }}
        placeholder='{ "initial": "…", "states": { … } }'
        spellCheck={false}
        aria-invalid={!!error}
        aria-describedby="definition-error"
      />
      {error && (
        <div id="definition-error" role="alert" className="alert alert-error py-2 font-mono text-xs">{error}</div>
      )}
      <button className="btn btn-primary" disabled={busy || !appId || !name || !definition.trim() || !!parseError} onClick={() => void publish()}>
        {busy && <span className="loading loading-spinner loading-sm" />}Publish version
      </button>
    </div>
  );
}

function StartScheduleForm({ appId }: { appId: string }) {
  const { send } = useAdmin();
  const { notify, appName, go, bump } = useShell();
  const { rows: versions } = useList<Version>("workflow_versions");
  const { rows: calendars } = useList<Calendar>("calendars");
  const workflows = versions.filter((v) => v.application_id === appId);
  const appCalendars = calendars.filter((c) => c.application_id === appId);
  const [name, setName] = useState("");
  const [workflow, setWorkflow] = useState("");
  const [calendar, setCalendar] = useState("");
  const [localTime, setLocalTime] = useState("09:00");
  const [policy, setPolicy] = useState("skip");
  const [busy, setBusy] = useState(false);

  async function save() {
    setBusy(true);
    try {
      const r = await send({ op: "start-schedule.set", applicationId: appId, name, workflow, calendar, localTime, missedPolicy: policy });
      notify(r.ok ? `Start schedule ${name} set for ${appName(appId)}` : r.error ?? "start-schedule.set failed", r.ok);
      if (r.ok) {
        bump();
        go("schedules");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="grid max-w-2xl gap-4 sm:grid-cols-2">
        <label className="form-control">
          <span className="label-text mb-1">Name</span>
          <input className="input input-bordered input-sm font-mono" value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="form-control">
          <span className="label-text mb-1">Workflow definition</span>
          <select className="select select-bordered select-sm" value={workflow} onChange={(e) => setWorkflow(e.target.value)}>
            <option value="">{workflows.length ? "Choose…" : "No published workflows"}</option>
            {workflows.map((w) => <option key={w.name} value={w.name}>{w.name} (v{w.version})</option>)}
          </select>
        </label>
        <label className="form-control">
          <span className="label-text mb-1">Business calendar</span>
          <select className="select select-bordered select-sm" value={calendar} onChange={(e) => setCalendar(e.target.value)}>
            <option value="">{appCalendars.length ? "Choose…" : "No calendars"}</option>
            {appCalendars.map((c) => <option key={c.name} value={c.name}>{c.name}</option>)}
          </select>
        </label>
        <label className="form-control">
          <span className="label-text mb-1">Local time</span>
          <input type="time" className="input input-bordered input-sm" value={localTime} onChange={(e) => setLocalTime(e.target.value)} />
        </label>
        <label className="form-control">
          <span className="label-text mb-1">Missed occurrence policy</span>
          <select className="select select-bordered select-sm" value={policy} onChange={(e) => setPolicy(e.target.value)}>
            <option value="skip">skip</option>
            <option value="run_once_late">run_once_late</option>
            <option value="catch_up">catch_up</option>
          </select>
        </label>
      </div>
      <button className="btn btn-primary" disabled={busy || !appId || !name || !workflow || !calendar} onClick={() => void save()}>
        {busy && <span className="loading loading-spinner loading-sm" />}Set start schedule
      </button>
    </div>
  );
}
