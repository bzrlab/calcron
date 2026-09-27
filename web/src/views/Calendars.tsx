import { useEffect, useMemo, useState } from "react";
import { useAdmin } from "../lib/admin";
import { useList } from "../lib/useList";
import { useShell } from "../lib/shell";
import { DataTable, type Column } from "../components/DataTable";
import { CalendarGrid } from "../components/CalendarGrid";
import { AppName, Section } from "../components/ui";
import { fmt } from "../lib/format";
import { BROWSER_ZONE, ZONES, cycleOverride, days, type CalendarDefinition as Definition, type Mode } from "../lib/calendar";
import { startEvent, useStartOccurrences } from "../lib/useStartOccurrences";
import type { StartSchedule } from "./Publish";
type Calendar = { id: string; application_id: string; name: string; definition: Definition; updated_at: string };

const DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
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

  if (current || selected === NEW) {
    return (
      <div className="space-y-4">
        <div className="flex items-center gap-3">
          <button className="btn btn-ghost btn-sm" onClick={() => select()}>← All calendars</button>
          <h2 className="text-xl font-semibold">{current ? current.name : "New Business calendar"}</h2>
          {current && <AppName id={current.application_id} />}
        </div>
        <CalendarEditor key={selected} calendar={current} onSaved={(c) => select(c.id)} />
      </div>
    );
  }

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
    </div>
  );
}

const canonical = (d: Definition) => JSON.stringify([d.timezone, [...d.weekdays].sort(), Object.entries(d.overrides ?? {}).sort()]);

function CalendarEditor({ calendar, onSaved }: { calendar?: Calendar; onSaved: (c: { id: string }) => void }) {
  const { send } = useAdmin();
  const { apps, notify, bump } = useShell();
  const [pickedApp, setAppId] = useState(calendar?.application_id ?? "");
  const appId = pickedApp || apps[0]?.id || "";
  const [name, setName] = useState(calendar?.name ?? "");
  const saved: Definition = calendar?.definition ?? { timezone: BROWSER_ZONE, weekdays: [1, 2, 3, 4, 5], overrides: {} };
  const [draft, setDraft] = useState<Definition>({ ...saved, overrides: saved.overrides ?? {} });
  const [mode, setMode] = useState<Mode>("month");
  const [anchor, setAnchor] = useState(() => new Date());
  const [busy, setBusy] = useState(false);
  const { rows: allStarts } = useList<StartSchedule>("start_schedules");
  const starts = useMemo(() => allStarts.filter((s) => calendar && s.application_id === calendar.application_id && s.calendar_name === calendar.name), [allStarts, calendar]);
  const occurrences = useStartOccurrences(starts, days(mode, anchor), "", draft);
  const events = useMemo(() => occurrences.map(({ s, at }) => startEvent(s, at)), [occurrences]);
  const dirty = !calendar || canonical(draft) !== canonical(saved);
  const overrides = Object.entries(draft.overrides ?? {}).sort(([a], [b]) => a.localeCompare(b));

  const toggleWeekday = (day: number) =>
    setDraft((d) => ({ ...d, weekdays: d.weekdays.includes(day) ? d.weekdays.filter((x) => x !== day) : [...d.weekdays, day].sort() }));
  const toggleDate = (day: string) => setDraft((d) => ({ ...d, overrides: cycleOverride(d, day) }));

  async function save() {
    setBusy(true);
    try {
      const r = await send({ op: "calendar.set", applicationId: appId, name, data: draft });
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
    <div className="grid items-start gap-6 lg:grid-cols-[18rem_1fr]">
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
        <select className="select select-bordered select-sm" value={draft.timezone} onChange={(e) => setDraft((d) => ({ ...d, timezone: e.target.value }))}>
          {ZONES.map((z) => <option key={z}>{z}</option>)}
        </select>
      </label>
      <Section title="Eligible weekdays">
        <div className="join">
          {DAYS.map((d, i) => (
            <button
              key={d}
              type="button"
              title={d}
              aria-pressed={draft.weekdays.includes(i)}
              className={`btn btn-sm join-item ${draft.weekdays.includes(i) ? "btn-success" : "btn-ghost border-base-300"}`}
              onClick={() => toggleWeekday(i)}
            >
              {d.slice(0, 2)}
            </button>
          ))}
        </div>
      </Section>
      <Section title={`Date overrides (${overrides.length})`}>
        {overrides.length === 0 && <p className="text-sm text-base-content/50">Click a day on the calendar to close it (holiday) or open it (extra working day).</p>}
        <ul className="max-h-56 space-y-1 overflow-y-auto">
          {overrides.map(([date, open]) => (
            <li key={date} className="flex items-center gap-2 text-sm">
              <span className="font-mono">{date}</span>
              <span className={`badge badge-sm ${open ? "badge-success" : "badge-error"}`}>{open ? "open" : "closed"}</span>
              <button type="button" className="btn btn-ghost btn-xs ml-auto" onClick={() => toggleDate(date)} aria-label={`Remove override ${date}`}>✕</button>
            </li>
          ))}
        </ul>
        <label className="flex items-center gap-2 text-xs text-base-content/60">
          Add date
          <input type="date" className="input input-bordered input-xs" value="" onChange={(e) => e.target.value && draft.overrides?.[e.target.value] === undefined && toggleDate(e.target.value)} />
        </label>
      </Section>
      <div className="flex items-center gap-2">
        <button className="btn btn-primary btn-sm" disabled={busy || !appId || !name || !dirty || draft.weekdays.length === 0} onClick={() => void save()}>
          {busy && <span className="loading loading-spinner loading-sm" />}Save calendar
        </button>
        {calendar && dirty && (
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setDraft({ ...saved, overrides: saved.overrides ?? {} })}>Discard</button>
        )}
        {calendar && dirty && <span className="text-xs text-warning">Unsaved changes</span>}
      </div>
      {draft.weekdays.length === 0 && <p className="text-xs text-error">A Business calendar needs at least one eligible weekday.</p>}
      {draft.weekdays.length > 0 && <NextDays definition={draft} />}
      </div>
      <div className="space-y-2">
        <p className="text-xs text-base-content/60">
          Click a day to flip it for that date only; click again to restore the weekday rule. Click a weekday name to change it every week.
          Hatched days are closed. Times are in {draft.timezone}; Start schedule runs on this calendar preview your unsaved changes. Nothing is saved until you press Save.
        </p>
        <CalendarGrid
          mode={mode}
          anchor={anchor}
          onMode={setMode}
          onAnchor={setAnchor}
          timeZone={draft.timezone}
          events={events}
          definition={draft}
          onDay={toggleDate}
          onWeekday={toggleWeekday}
        />
      </div>
    </div>
  );
}

// NextDays previews the draft with the server's own calendar arithmetic,
// so what it shows is exactly what a Start schedule would use once saved.
function NextDays({ definition }: { definition: Definition }) {
  const { send } = useAdmin();
  const [localTime, setLocalTime] = useState("09:00");
  const [runs, setRuns] = useState<string[] | string>([]);
  const timezone = definition.timezone;
  const key = JSON.stringify(definition);

  useEffect(() => {
    let cancelled = false;
    const at = new Date();
    const until = new Date(at.getTime() + 365 * 86_400_000);
    void send({ op: "calendar.occurrences", data: definition, localTime, at: at.toISOString(), until: until.toISOString() }).then((r) => {
      if (!cancelled) setRuns(r.ok ? r.data.occurrences.slice(0, 5) : r.error);
    });
    return () => { cancelled = true; };
  }, [send, key, localTime]);

  return (
    <Section title={`Next 5 eligible days (${timezone})`}>
      <label className="flex items-center gap-2 text-sm">
        at local time
        <input type="time" className="input input-bordered input-xs" value={localTime} onChange={(e) => setLocalTime(e.target.value)} />
      </label>
      {typeof runs === "string" ? (
        <p className="text-sm text-error">{runs}</p>
      ) : (
        <ul className="space-y-1 text-sm">
          {runs.map((d) => (
            <li key={d} className="font-mono">
              {new Date(d).toLocaleString(undefined, { timeZone: timezone, weekday: "short", year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}
            </li>
          ))}
        </ul>
      )}
    </Section>
  );
}
