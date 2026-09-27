import { useEffect, useMemo, useState } from "react";
import { useAdmin } from "../lib/admin";
import { useList } from "../lib/useList";
import { useShell } from "../lib/shell";
import { DataTable, type Column } from "../components/DataTable";
import { AppName, Drawer, Section } from "../components/ui";
import { fmt } from "../lib/format";

type Definition = { timezone: string; weekdays: number[]; overrides?: Record<string, boolean> };
type Calendar = { id: string; application_id: string; name: string; definition: Definition; updated_at: string };

const DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
const ZONES = Intl.supportedValuesOf("timeZone");
const NEW = "new";

export function Calendars() {
  const { rows: raw, loading } = useList<Omit<Calendar, "id">>("calendars");
  const { selected, select } = useShell();
  const rows = useMemo(() => raw.map((c) => ({ ...c, id: `${c.application_id}:${c.name}` })), [raw]);
  const current = rows.find((c) => c.id === selected);

  const columns: Column<Calendar>[] = [
    { key: "name", label: "Business calendar", render: (c) => <span className="font-medium">{c.name}</span> },
    { key: "app", label: "Application", render: (c) => <AppName id={c.application_id} /> },
    { key: "tz", label: "Timezone", render: (c) => <code className="text-xs">{c.definition?.timezone ?? "—"}</code> },
    {
      key: "days",
      label: "Eligible days",
      render: (c) => (
        <div className="flex gap-1">
          {DAYS.map((d, i) => (
            <span key={d} className={`badge badge-sm ${c.definition?.weekdays?.includes(i) ? "badge-success" : "badge-ghost"}`} title={d}>
              {d[0]}
            </span>
          ))}
        </div>
      ),
    },
    { key: "overrides", label: "Overrides", render: (c) => <span className="tabular-nums">{Object.keys(c.definition?.overrides ?? {}).length}</span> },
    { key: "updated", label: "Updated", render: (c) => <span className="text-xs text-base-content/60">{fmt(c.updated_at)}</span> },
  ];

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Calendars</h2>
          <p className="text-sm text-base-content/60">Named timezone-aware Business calendars: eligible weekdays and date overrides.</p>
        </div>
        <button className="btn btn-primary btn-sm" onClick={() => select(NEW)}>+ New calendar</button>
      </header>
      <DataTable columns={columns} rows={rows} loading={loading} empty="No calendars." onRowClick={(c) => select(c.id)} selectedId={selected} />

      {(current || selected === NEW) && (
        <Drawer title={current ? current.name : "New Business calendar"} onClose={() => select()}>
          <CalendarForm key={selected} calendar={current} onSaved={(c) => select(c.id)} />
        </Drawer>
      )}
    </div>
  );
}

function CalendarForm({ calendar, onSaved }: { calendar?: Calendar; onSaved: (c: { id: string }) => void }) {
  const { send } = useAdmin();
  const { apps, notify, bump } = useShell();
  const [pickedApp, setAppId] = useState(calendar?.application_id ?? "");
  const appId = pickedApp || apps[0]?.id || "";
  const [name, setName] = useState(calendar?.name ?? "");
  const [timezone, setTimezone] = useState(calendar?.definition.timezone ?? Intl.DateTimeFormat().resolvedOptions().timeZone);
  const [weekdays, setWeekdays] = useState<number[]>(calendar?.definition.weekdays ?? [1, 2, 3, 4, 5]);
  const [overrides, setOverrides] = useState<[string, boolean][]>(Object.entries(calendar?.definition.overrides ?? {}));
  const [busy, setBusy] = useState(false);

  function toggle(day: number) {
    setWeekdays((w) => (w.includes(day) ? w.filter((d) => d !== day) : [...w, day].sort()));
  }

  async function save() {
    setBusy(true);
    try {
      const data: Definition = { timezone, weekdays, overrides: Object.fromEntries(overrides.filter(([d]) => d)) };
      const r = await send({ op: "calendar.set", applicationId: appId, name, data });
      notify(r.ok ? `Saved calendar ${name}` : r.error ?? "calendar.set failed", r.ok);
      if (r.ok) {
        bump();
        onSaved({ id: `${appId}:${name}` });
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-5">
      {!calendar && (
        <div className="grid gap-3">
          <label className="form-control">
            <span className="label-text mb-1">Application</span>
            <select className="select select-bordered select-sm" value={appId} onChange={(e) => setAppId(e.target.value)}>
              {apps.map((a) => <option key={a.id} value={a.id}>{a.namespace}</option>)}
            </select>
          </label>
          <label className="form-control">
            <span className="label-text mb-1">Name</span>
            <input className="input input-bordered input-sm font-mono" value={name} onChange={(e) => setName(e.target.value)} />
          </label>
        </div>
      )}
      <label className="form-control">
        <span className="label-text mb-1">Timezone</span>
        <select className="select select-bordered select-sm" value={timezone} onChange={(e) => setTimezone(e.target.value)}>
          {ZONES.map((z) => <option key={z}>{z}</option>)}
        </select>
      </label>
      <Section title="Eligible weekdays">
        <div className="space-y-1">
          {DAYS.map((d, i) => (
            <label key={d} className="flex cursor-pointer items-center gap-3 py-1">
              <input type="checkbox" className="toggle toggle-sm toggle-success" checked={weekdays.includes(i)} onChange={() => toggle(i)} />
              <span className={weekdays.includes(i) ? "" : "text-base-content/40"}>{d}</span>
            </label>
          ))}
        </div>
      </Section>
      <Section title="Date overrides">
        {overrides.length === 0 && <p className="text-sm text-base-content/50">Holidays or special open days replace the weekday rule for that date.</p>}
        {overrides.map(([date, open], i) => (
          <div key={i} className="flex items-center gap-2">
            <input
              type="date"
              className="input input-bordered input-sm"
              value={date}
              onChange={(e) => setOverrides((o) => o.map((x, j) => (j === i ? [e.target.value, x[1]] : x)))}
            />
            <select
              className="select select-bordered select-sm"
              value={open ? "open" : "closed"}
              onChange={(e) => setOverrides((o) => o.map((x, j) => (j === i ? [x[0], e.target.value === "open"] : x)))}
            >
              <option value="closed">closed</option>
              <option value="open">open</option>
            </select>
            <button className="btn btn-ghost btn-xs" onClick={() => setOverrides((o) => o.filter((_, j) => j !== i))} aria-label="Remove override">✕</button>
          </div>
        ))}
        <button className="btn btn-ghost btn-xs" onClick={() => setOverrides((o) => [...o, ["", false]])}>+ Add override</button>
      </Section>
      <button className="btn btn-primary btn-sm" disabled={busy || !appId || !name} onClick={() => void save()}>
        {busy && <span className="loading loading-spinner loading-sm" />}Save calendar
      </button>
      {calendar && (
        <NextDays appId={calendar.application_id} name={calendar.name} timezone={calendar.definition.timezone} version={calendar.updated_at} />
      )}
    </div>
  );
}

// NextDays previews the saved calendar with the server's own calendar.next,
// so what it shows is exactly what a Start schedule would use.
function NextDays({ appId, name, timezone, version }: { appId: string; name: string; timezone: string; version: string }) {
  const { send } = useAdmin();
  const [localTime, setLocalTime] = useState("09:00");
  const [days, setDays] = useState<string[] | string>([]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const out: string[] = [];
      let at: string | undefined;
      for (let i = 0; i < 5; i++) {
        const r = await send({ op: "calendar.next", applicationId: appId, calendar: name, localTime, ...(at ? { at } : {}) });
        if (!r.ok) {
          if (!cancelled) setDays(r.error ?? "calendar.next failed");
          return;
        }
        at = (r.data as { nextAt: string }).nextAt;
        out.push(at);
      }
      if (!cancelled) setDays(out);
    })();
    return () => { cancelled = true; };
  }, [send, appId, name, localTime, version]);

  return (
    <Section title={`Next 5 eligible days (${timezone})`}>
      <label className="flex items-center gap-2 text-sm">
        at local time
        <input type="time" className="input input-bordered input-xs" value={localTime} onChange={(e) => setLocalTime(e.target.value)} />
      </label>
      {typeof days === "string" ? (
        <p className="text-sm text-error">{days}</p>
      ) : (
        <ul className="space-y-1 text-sm">
          {days.map((d) => (
            <li key={d} className="font-mono">
              {new Date(d).toLocaleString(undefined, { timeZone: timezone, weekday: "short", year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}
            </li>
          ))}
        </ul>
      )}
    </Section>
  );
}
