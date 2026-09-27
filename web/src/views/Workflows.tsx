import { useMemo, useState } from "react";
import { useList } from "../lib/useList";
import { useShell } from "../lib/shell";
import { waitingReason } from "../lib/dashboard";
import { DataTable, type Column } from "../components/DataTable";
import { StatusBadge } from "../components/StatusBadge";
import { AppName, Chips, Drawer, Fields, IdChip, Pre, Section, SubjectHistory } from "../components/ui";
import { fmt, timeAgo } from "../lib/format";

type Instance = {
  id: string;
  application_id: string;
  workflow_name: string;
  workflow_version: number;
  current_state: string;
  status: string;
  waiting_event: string | null;
  correlation_key: string | null;
  wake_at: string | null;
  input: unknown;
  state: unknown;
  updated_at: string;
};

type Version = { application_id: string; name: string; version: number };

type Filter = "all" | "waiting" | "running" | "completed" | "cancelled";

function group(status: string): Filter {
  return status.startsWith("waiting_") ? "waiting" : (status as Filter);
}

export function Workflows() {
  const { rows, loading } = useList<Instance>("workflows");
  const { rows: versions } = useList<Version>("workflow_versions");
  const { selected, select } = useShell();
  const [filter, setFilter] = useState<Filter>("all");

  const latest = useMemo(() => new Map(versions.map((v) => [`${v.application_id}/${v.name}`, v.version])), [versions]);
  const counts = useMemo(() => {
    const c: Record<string, number> = { all: rows.length };
    for (const w of rows) c[group(w.status)] = (c[group(w.status)] ?? 0) + 1;
    return c;
  }, [rows]);
  const visible = filter === "all" ? rows : rows.filter((w) => group(w.status) === filter);
  const current = rows.find((w) => w.id === selected);

  function version(w: Instance) {
    const newest = latest.get(`${w.application_id}/${w.workflow_name}`);
    return (
      <span className="badge badge-ghost badge-sm font-mono" title={newest && newest > w.workflow_version ? "Pinned to an older version" : undefined}>
        v{w.workflow_version}
        {newest && newest > w.workflow_version && <span className="text-warning"> · latest v{newest}</span>}
      </span>
    );
  }

  const columns: Column<Instance>[] = [
    { key: "name", label: "Workflow", render: (w) => <span className="font-medium">{w.workflow_name}</span> },
    { key: "version", label: "Version", render: version },
    {
      key: "status",
      label: "Status",
      render: (w) => (
        <div className="flex flex-col gap-0.5">
          <StatusBadge value={w.status} />
          {waitingReason(w) && <span className="font-mono text-xs text-base-content/60">{waitingReason(w)}</span>}
        </div>
      ),
    },
    { key: "state", label: "Current state", render: (w) => <code className="text-xs">{w.current_state}</code> },
    { key: "app", label: "Application", render: (w) => <AppName id={w.application_id} /> },
    { key: "updated", label: "Updated", render: (w) => <span title={fmt(w.updated_at)}>{timeAgo(w.updated_at)}</span> },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-xl font-semibold">Workflows</h2>
        <p className="text-sm text-base-content/60">Workflow instances pinned to an immutable Workflow version.</p>
      </header>
      <Chips<Filter>
        value={filter}
        onChange={setFilter}
        options={(["all", "waiting", "running", "completed", "cancelled"] as Filter[]).map((f) => ({
          value: f,
          label: f[0].toUpperCase() + f.slice(1),
          count: counts[f] ?? 0,
        }))}
      />
      <DataTable
        columns={columns}
        rows={visible}
        loading={loading}
        empty="No Workflow instances. They start from an application SDK or a Start schedule."
        onRowClick={(w) => select(w.id)}
        selectedId={selected}
      />

      {current && (
        <Drawer title={`${current.workflow_name} · ${current.current_state}`} onClose={() => select()}>
          <Fields
            rows={[
              ["Workflow instance ID", <IdChip value={current.id} />],
              ["Version", version(current)],
              ["Status", <StatusBadge value={current.status} />],
              ["Waiting for", waitingReason(current) || "—"],
              ["Correlation key", current.correlation_key ? <code className="text-xs">{current.correlation_key}</code> : "—"],
              ["Wakes at", current.wake_at ? fmt(current.wake_at) : "—"],
              ["Application", <AppName id={current.application_id} />],
            ]}
          />
          <Section title="State timeline"><SubjectHistory type="workflow" id={current.id} /></Section>
          <Section title="Input"><Pre value={current.input} /></Section>
          <Section title="Workflow state"><Pre value={current.state} /></Section>
        </Drawer>
      )}
    </div>
  );
}
