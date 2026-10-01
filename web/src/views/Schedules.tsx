import { useMemo, useState } from "react";
import { useList } from "../lib/useList";
import { useShell } from "../lib/shell";
import { dueBucket, type DueBucket } from "../lib/dashboard";
import { DataTable, type Column } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { AppName, Chips, Drawer, Fields, IdChip, Pre, Section, SubjectHistory } from "../components/ui";
import { fmt, timeAgo } from "../lib/format";

type Schedule = {
  id: string;
  application_id: string;
  schedule_key: string;
  event: string;
  payload: unknown;
  chain: unknown;
  run_at: string;
  status: string;
  updated_at: string;
};

type StartSchedule = {
  id: string;
  application_id: string;
  name: string;
  workflow_name: string;
  calendar_name: string;
  local_time: string;
  missed_policy: string;
  next_at: string;
  status: string;
};

const BUCKETS: { key: DueBucket; label: string; tone: string }[] = [
  { key: "overdue", label: "Overdue", tone: "text-error" },
  { key: "hour", label: "Next hour", tone: "text-warning" },
  { key: "today", label: "Today", tone: "" },
  { key: "later", label: "Later", tone: "" },
  { key: "done", label: "Delivered / cancelled", tone: "text-base-content/50" },
];

type Tab = "schedules" | "start";

export function Schedules() {
  const [tab, setTab] = useState<Tab>("schedules");
  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold">Schedules</h2>
          <p className="text-sm text-base-content/60">
            One named future delivery per Schedule key. Applications own cancellation.
          </p>
        </div>
        <Chips<Tab>
          value={tab}
          onChange={setTab}
          options={[{ value: "schedules", label: "Schedules" }, { value: "start", label: "Start schedules" }]}
        />
      </header>
      {tab === "schedules" ? <ScheduleBuckets /> : <StartSchedules />}
    </div>
  );
}

function ScheduleBuckets() {
  const { rows, loading } = useList<Schedule>("schedules");
  const { selected, select, apps } = useShell();
  const [app, setApp] = useState("");
  const [showDone, setShowDone] = useState(false);

  const visible = useMemo(() => rows.filter((s) => !app || s.application_id === app), [rows, app]);
  const grouped = useMemo(() => {
    const now = new Date();
    const out = new Map<DueBucket, Schedule[]>(BUCKETS.map((b) => [b.key, []]));
    for (const s of visible) out.get(dueBucket(s.run_at, s.status, now))!.push(s);
    for (const [key, list] of out) {
      list.sort((a, b) => (key === "done" ? b.run_at.localeCompare(a.run_at) : a.run_at.localeCompare(b.run_at)));
    }
    return out;
  }, [visible]);
  const next = [...(grouped.get("hour") ?? []), ...(grouped.get("today") ?? []), ...(grouped.get("later") ?? [])][0];
  const current = rows.find((s) => s.id === selected);

  const columns: Column<Schedule>[] = [
    { key: "key", label: "Schedule key", render: (s) => <code className="text-xs">{s.schedule_key}</code> },
    { key: "event", label: "Event", render: (s) => <span className="font-medium">{s.event}</span> },
    { key: "app", label: "Application", render: (s) => <AppName id={s.application_id} /> },
    { key: "run", label: "Due", render: (s) => <span title={fmt(s.run_at)}>{timeAgo(s.run_at)}</span> },
    { key: "status", label: "Status", render: (s) => <StatusBadge value={s.status} /> },
  ];

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3">
        {next ? (
          <div className="rounded-box border border-primary/40 bg-primary/10 px-4 py-2 text-sm">
            Next delivery <strong>{timeAgo(next.run_at)}</strong> · <code>{next.schedule_key}</code> · {next.event}
          </div>
        ) : (
          <div className="text-sm text-base-content/50">Nothing scheduled ahead.</div>
        )}
        <select className="select select-sm select-bordered" value={app} onChange={(e) => setApp(e.target.value)} aria-label="Filter by application">
          <option value="">All applications</option>
          {apps.map((a) => <option key={a.id} value={a.id}>{a.namespace}</option>)}
        </select>
      </div>

      {BUCKETS.map((b) => {
        const list = grouped.get(b.key) ?? [];
        if (list.length === 0 && b.key !== "done") return null;
        const collapsed = b.key === "done" && !showDone;
        return (
          <section key={b.key} className="space-y-2">
            <h3 className={`flex items-center gap-2 text-sm font-medium ${b.tone}`}>
              {b.label} <span className="badge badge-sm badge-ghost tabular-nums">{list.length}</span>
              {b.key === "done" && list.length > 0 && (
                <button className="btn btn-ghost btn-xs" onClick={() => setShowDone((v) => !v)}>{showDone ? "hide" : "show"}</button>
              )}
            </h3>
            {!collapsed && (
              <DataTable columns={columns} rows={list} loading={loading} empty="No schedules." onRowClick={(s) => select(s.id)} selectedId={selected} />
            )}
          </section>
        );
      })}
      {!loading && rows.length === 0 && (
        <p className="text-sm text-base-content/50">No schedules yet. Applications create them with <code>schedule.set</code>.</p>
      )}

      {current && (
        <Drawer title={<code>{current.schedule_key}</code>} onClose={() => select()}>
          <Fields
            rows={[
              ["Schedule ID", <IdChip value={current.id} />],
              ["Event", current.event],
              ["Application", <AppName id={current.application_id} />],
              ["Status", <StatusBadge value={current.status} />],
              ["Due (local)", fmt(current.run_at)],
              ["Due (UTC)", <code className="text-xs">{current.run_at}</code>],
            ]}
          />
          <Section title="Payload"><Pre value={current.payload} /></Section>
          {current.chain != null && <Section title="Chain"><Pre value={current.chain} /></Section>}
          <Section title="History"><SubjectHistory type="schedule" id={current.id} /></Section>
        </Drawer>
      )}
    </>
  );
}

function StartSchedules() {
  const { rows, loading } = useList<StartSchedule>("start_schedules");
  const columns: Column<StartSchedule>[] = [
    { key: "name", label: "Start schedule", render: (s) => <span className="font-medium">{s.name}</span> },
    { key: "wf", label: "Workflow", render: (s) => <code className="text-xs">{s.workflow_name}</code> },
    { key: "cal", label: "Business calendar", render: (s) => <code className="text-xs">{s.calendar_name}</code> },
    { key: "time", label: "Local time", render: (s) => <span className="tabular-nums">{s.local_time}</span> },
    { key: "policy", label: "Missed occurrence policy", render: (s) => <code className="text-xs">{s.missed_policy}</code> },
    { key: "next", label: "Next start", render: (s) => <span title={fmt(s.next_at)}>{timeAgo(s.next_at)}</span> },
    { key: "status", label: "Status", render: (s) => <StatusBadge value={s.status} /> },
    { key: "app", label: "Application", render: (s) => <AppName id={s.application_id} /> },
  ];
  return <DataTable columns={columns} rows={rows} loading={loading} empty="No start schedules. Set one from Publish." />;
}
