import { useEffect, useState, type ReactNode } from "react";
import { useAdmin } from "../lib/admin";
import { useShell } from "../lib/shell";
import { fmt } from "../lib/format";

export function IdChip({ value, onOpen }: { value: string; onOpen?: () => void }) {
  const [copied, setCopied] = useState(false);
  async function copy(e: React.MouseEvent) {
    e.stopPropagation();
    await navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  }
  const label = value.length > 12 ? `${value.slice(0, 10)}…` : value;
  return (
    <span className="inline-flex items-center gap-1 rounded bg-base-300 px-1.5 py-0.5 font-mono text-xs" title={value}>
      {onOpen ? (
        <button type="button" className="link-hover" onClick={(e) => { e.stopPropagation(); onOpen(); }}>{label}</button>
      ) : (
        label
      )}
      <button type="button" className="opacity-60 hover:opacity-100" onClick={copy} aria-label={`Copy ${value}`}>
        {copied ? "✓" : "⧉"}
      </button>
    </span>
  );
}

export function AppName({ id }: { id: string }) {
  const { apps, appName } = useShell();
  const app = apps.find((a) => a.id === id);
  return (
    <span className="inline-flex items-center gap-1.5 text-sm" title={id}>
      <span
        className={`inline-block h-1.5 w-1.5 rounded-full ${app?.connected ? "bg-success" : "bg-base-content/30"}`}
        aria-label={app?.connected ? "connected" : "offline"}
      />
      {appName(id)}
    </span>
  );
}

export function Drawer({ title, onClose, children }: { title: ReactNode; onClose: () => void; children: ReactNode }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <aside
      className="fixed inset-y-0 right-0 z-40 flex w-full max-w-md flex-col border-l border-base-300 bg-base-100 shadow-2xl"
      aria-label="Detail"
    >
      <header className="flex items-center justify-between gap-2 border-b border-base-300 px-5 py-3">
        <h3 className="truncate font-semibold">{title}</h3>
        <button className="btn btn-ghost btn-sm btn-square" onClick={onClose} aria-label="Close detail">✕</button>
      </header>
      <div className="flex-1 space-y-5 overflow-y-auto p-5">{children}</div>
    </aside>
  );
}

export function Fields({ rows }: { rows: [string, ReactNode][] }) {
  return (
    <dl className="grid grid-cols-[8rem_1fr] gap-x-3 gap-y-2 text-sm">
      {rows.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-base-content/50">{k}</dt>
          <dd className="min-w-0 break-words">{v ?? "—"}</dd>
        </div>
      ))}
    </dl>
  );
}

export function Chips<T extends string>({
  options,
  value,
  onChange,
}: {
  options: { value: T; label: string; count?: number }[];
  value: T;
  onChange: (v: T) => void;
}) {
  return (
    <div className="flex flex-wrap gap-1" role="group">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={value === o.value}
          className={`btn btn-xs ${value === o.value ? "btn-primary" : "btn-ghost border-base-300"}`}
          onClick={() => onChange(o.value)}
        >
          {o.label}
          {o.count !== undefined && <span className="opacity-70 tabular-nums">{o.count}</span>}
        </button>
      ))}
    </div>
  );
}

type HistoryRow = { id: number; event: string; data: unknown; created_at: string };

export function SubjectHistory({ type, id }: { type: string; id: string }) {
  const { send } = useAdmin();
  const { refresh } = useShell();
  const [rows, setRows] = useState<HistoryRow[] | null>(null);
  useEffect(() => {
    void send({ op: "dashboard.list", name: "history", subjectType: type, subjectId: id }).then((r) => {
      if (r.ok) setRows(((r.data as HistoryRow[]) ?? []).slice().reverse());
    });
  }, [send, type, id, refresh]);
  if (!rows) return <span className="loading loading-spinner loading-sm" />;
  if (rows.length === 0) return <p className="text-sm text-base-content/50">No history.</p>;
  return (
    <ol className="relative space-y-3 border-l border-base-300 pl-4">
      {rows.map((h) => (
        <li key={h.id} className="relative">
          <span className="absolute -left-[1.3rem] top-1.5 h-2 w-2 rounded-full bg-primary" />
          <div className="flex items-baseline justify-between gap-2">
            <span className="font-mono text-sm">{h.event}</span>
            <span className="text-xs text-base-content/50">{fmt(h.created_at)}</span>
          </div>
          {h.data != null && Object.keys(h.data as object).length > 0 && (
            <pre className="mt-1 max-h-40 overflow-auto rounded bg-base-300 p-2 text-xs">{JSON.stringify(h.data, null, 2)}</pre>
          )}
        </li>
      ))}
    </ol>
  );
}

export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-2">
      <h4 className="text-xs uppercase tracking-wider text-base-content/50">{title}</h4>
      {children}
    </section>
  );
}

export function Pre({ value }: { value: unknown }) {
  return (
    <pre className="max-h-64 overflow-auto rounded-box bg-base-300 p-3 text-xs leading-relaxed">
      {JSON.stringify(value ?? null, null, 2)}
    </pre>
  );
}
