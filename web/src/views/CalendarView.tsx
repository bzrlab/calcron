import { useMemo, useState } from "react";
import { useAdmin } from "../lib/admin";
import { useList } from "../lib/useList";
import { useShell } from "../lib/shell";
import { BROWSER_ZONE, ZONES, days, hhmm, projectMoveIntent, type CalendarDefinition, type CalendarMoveIntent, type Mode } from "../lib/calendar";
import { startEvent, useStartOccurrences } from "../lib/useStartOccurrences";
import { canCancel, canReplay, useDeliveryActions } from "../lib/useDeliveryActions";
import { fmt } from "../lib/format";
import { CalendarGrid, type GridEvent, type Tone } from "../components/CalendarGrid";
import { StatusBadge } from "../components/StatusBadge";
import { AppName, Drawer, Fields, IdChip, Pre } from "../components/ui";
import { StartScheduleForm, type StartSchedule } from "./Publish";

type Schedule = { id: string; application_id: string; schedule_key: string; event: string; payload: unknown; run_at: string; status: string };
type Delivery = { id: string; application_id: string; event: string; status: string; attempts: number; next_attempt_at: string };
type Workflow = { id: string; application_id: string; workflow_name: string; current_state: string; status: string; wake_at: string | null };
type BusinessCalendar = { application_id: string; name: string; definition: CalendarDefinition; updated_at: string };

type Kind = "schedule" | "delivery" | "workflow" | "start";
type Item = GridEvent & { kind: Kind; id: string };

const LAYERS: { kind: Kind; label: string; tone: Tone }[] = [
  { kind: "schedule", label: "Schedules", tone: "info" },
  { kind: "delivery", label: "Deliveries", tone: "warning" },
  { kind: "workflow", label: "Workflow wakes", tone: "success" },
  { kind: "start", label: "Start schedules", tone: "primary" },
];
const DOT: Record<Tone, string> = {
  info: "bg-info", warning: "bg-warning", error: "bg-error", success: "bg-success", primary: "bg-primary", neutral: "bg-base-content/30",
};

export function CalendarView() {
  const { selected, select, go, apps, appName, confirm, notify, bump } = useShell();
  const { send } = useAdmin();
  const [mode, setMode] = useState<Mode>("month");
  const [anchor, setAnchor] = useState(() => new Date());
  const [layers, setLayers] = useState<Kind[]>(LAYERS.map((l) => l.kind));
  const [app, setApp] = useState("");
  const [shade, setShade] = useState("");
  const [timeZone, setTimeZone] = useState(BROWSER_ZONE);
  const { rows: schedules } = useList<Schedule>("schedules");
  const { rows: deliveries, reload } = useList<Delivery>("deliveries");
  const { rows: workflows } = useList<Workflow>("workflows");
  const { rows: starts } = useList<StartSchedule>("start_schedules");
  const { rows: calendars } = useList<BusinessCalendar>("calendars");
  const { replay, cancel } = useDeliveryActions(() => void reload());

  const shown = days(mode, anchor);
  const calendarVersion = calendars.map((c) => c.updated_at).join();
  const occurrences = useStartOccurrences(starts, shown, calendarVersion);

  const items = useMemo(() => {
    const out: Item[] = [];
    const want = (kind: Kind, appId: string) => layers.includes(kind) && (!app || appId === app);
    for (const s of schedules) {
      if (want("schedule", s.application_id)) {
        out.push({ key: `schedule:${s.id}`, kind: "schedule", id: s.id, at: new Date(s.run_at), label: s.event, tone: s.status === "scheduled" ? "info" : "neutral" });
      }
    }
    for (const d of deliveries) {
      if ((d.status === "pending" || d.status === "blocked") && want("delivery", d.application_id)) {
        out.push({ key: `delivery:${d.id}`, kind: "delivery", id: d.id, at: new Date(d.next_attempt_at), label: d.event, tone: d.status === "blocked" ? "error" : "warning" });
      }
    }
    for (const w of workflows) {
      if (w.status === "waiting_time" && w.wake_at && want("workflow", w.application_id)) {
        out.push({ key: `workflow:${w.id}`, kind: "workflow", id: w.id, at: new Date(w.wake_at), label: `${w.workflow_name} wakes`, tone: "success" });
      }
    }
    for (const { s, at } of occurrences) {
      if (want("start", s.application_id)) {
        out.push({ ...startEvent(s, at), kind: "start", id: s.id, movable: true });
      }
    }
    return out;
  }, [schedules, deliveries, workflows, occurrences, layers, app]);

  const shadeCalendar = calendars.find((c) => `${c.application_id}:${c.name}` === shade);
  const current = items.find((i) => i.key === selected);
  const toggleLayer = (k: Kind) => setLayers((l) => (l.includes(k) ? l.filter((x) => x !== k) : [...l, k]));

  // A Start schedule runs at one local time in its Business calendar's zone, so a drop sets that time for every future run.
  async function move(intent: CalendarMoveIntent) {
    const s = starts.find((x) => x.id === items.find((i) => i.key === intent.eventKey)?.id);
    const zone = calendars.find((c) => c.application_id === s?.application_id && c.name === s?.calendar_name)?.definition.timezone;
    if (!s || !zone) return;
    const localTime = hhmm(projectMoveIntent(intent, zone).minutes);
    if (localTime === s.local_time) return;
    const yes = await confirm({
      title: `Move ${s.name} to ${localTime}?`,
      body: `Every future run of this Start schedule will start at ${localTime} ${zone} (was ${s.local_time}). Its days stay as its Business calendar decides.`,
      action: "Move start schedule",
    });
    if (!yes) return;
    const r = await send({
      op: "start-schedule.set",
      applicationId: s.application_id,
      name: s.name,
      workflow: s.workflow_name,
      calendar: s.calendar_name,
      localTime,
      missedPolicy: s.missed_policy,
      data: s.input,
    });
    notify(r.ok ? `Start schedule ${s.name} moved to ${localTime} ${zone}` : r.error ?? "start-schedule.set failed", r.ok);
    if (r.ok) bump();
  }

  return (
    <div className="space-y-4">
      <header>
        <h2 className="text-xl font-semibold">Calendar</h2>
        <p className="text-sm text-base-content/60">
          Everything due, shown in the chosen timezone. Hatched days are closed in the chosen Business calendar. In week or day view, drag a Start schedule run to change its time.
        </p>
      </header>
      <div className="flex flex-wrap items-center gap-2">
        {LAYERS.map((l) => (
          <button
            key={l.kind}
            type="button"
            aria-pressed={layers.includes(l.kind)}
            className={`btn btn-xs gap-1.5 ${layers.includes(l.kind) ? "btn-active" : "btn-ghost opacity-50"}`}
            onClick={() => toggleLayer(l.kind)}
          >
            <span className={`h-2 w-2 rounded-full ${DOT[l.tone]}`} />
            {l.label}
          </button>
        ))}
        <select className="select select-bordered select-xs ml-auto" value={app} onChange={(e) => setApp(e.target.value)} aria-label="Filter by application">
          <option value="">All applications</option>
          {apps.map((a) => <option key={a.id} value={a.id}>{a.namespace}</option>)}
        </select>
        <select className="select select-bordered select-xs" value={shade} onChange={(e) => setShade(e.target.value)} aria-label="Shade closed days of a Business calendar">
          <option value="">No Business calendar shading</option>
          {calendars.map((c) => (
            <option key={`${c.application_id}:${c.name}`} value={`${c.application_id}:${c.name}`}>{appName(c.application_id)} / {c.name}</option>
          ))}
        </select>
        <select className="select select-bordered select-xs" value={timeZone} onChange={(e) => setTimeZone(e.target.value)} aria-label="Display timezone">
          <option value={BROWSER_ZONE}>{BROWSER_ZONE} (browser)</option>
          {shadeCalendar && shadeCalendar.definition.timezone !== BROWSER_ZONE && (
            <option value={shadeCalendar.definition.timezone}>{shadeCalendar.definition.timezone} ({shadeCalendar.name})</option>
          )}
          <option value="UTC">UTC</option>
          {ZONES.filter((z) => z !== BROWSER_ZONE && z !== "UTC").map((z) => <option key={z} value={z}>{z}</option>)}
        </select>
      </div>

      <CalendarGrid
        mode={mode}
        anchor={anchor}
        onMode={setMode}
        onAnchor={setAnchor}
        timeZone={timeZone}
        events={items}
        selectedKey={selected}
        onEvent={(key) => select(key)}
        onMove={(intent) => void move(intent)}
        definition={shadeCalendar?.definition}
      />

      {current && (
        <Drawer title={current.label} onClose={() => select()}>
          {current.kind === "schedule" && <ScheduleDetail tz={timeZone} s={schedules.find((s) => s.id === current.id)!} open={() => go("schedules", current.id)} />}
          {current.kind === "delivery" && (
            <DeliveryDetail tz={timeZone} d={deliveries.find((d) => d.id === current.id)!} open={() => go("deliveries", current.id)} replay={replay} cancel={cancel} />
          )}
          {current.kind === "workflow" && <WorkflowDetail tz={timeZone} w={workflows.find((w) => w.id === current.id)!} open={() => go("workflows", current.id)} />}
          {current.kind === "start" && <StartDetail tz={timeZone} s={starts.find((s) => s.id === current.id)!} at={current.at} />}
        </Drawer>
      )}
    </div>
  );
}

function ScheduleDetail({ s, open, tz }: { s: Schedule; open: () => void; tz: string }) {
  return (
    <>
      <Fields
        rows={[
          ["Due", fmt(s.run_at, tz)],
          ["Status", <StatusBadge value={s.status} />],
          ["Schedule key", <code className="text-xs">{s.schedule_key}</code>],
          ["Application", <AppName id={s.application_id} />],
          ["Schedule ID", <IdChip value={s.id} />],
        ]}
      />
      <Pre value={s.payload} />
      <p className="text-xs text-base-content/50">Only the owning application can move or cancel a Schedule.</p>
      <button type="button" className="btn btn-sm" onClick={open}>Open in Schedules</button>
    </>
  );
}

function DeliveryDetail({ d, open, replay, cancel, tz }: { d: Delivery; open: () => void; tz: string; replay: (d: Delivery) => Promise<void>; cancel: (d: Delivery) => Promise<void> }) {
  return (
    <>
      <div className="flex gap-2">
        <button type="button" className="btn btn-primary btn-sm" disabled={!canReplay(d)} onClick={() => void replay(d)}>Replay</button>
        <button type="button" className="btn btn-outline btn-error btn-sm" disabled={!canCancel(d)} onClick={() => void cancel(d)}>Cancel</button>
        <button type="button" className="btn btn-sm ml-auto" onClick={open}>Open in Deliveries</button>
      </div>
      <Fields
        rows={[
          ["Next attempt", fmt(d.next_attempt_at, tz)],
          ["Status", <StatusBadge value={d.status} />],
          ["Attempts", d.attempts],
          ["Application", <AppName id={d.application_id} />],
          ["Delivery ID", <IdChip value={d.id} />],
        ]}
      />
    </>
  );
}

function WorkflowDetail({ w, open, tz }: { w: Workflow; open: () => void; tz: string }) {
  return (
    <>
      <Fields
        rows={[
          ["Wakes at", w.wake_at ? fmt(w.wake_at, tz) : "—"],
          ["State", <code className="text-xs">{w.current_state}</code>],
          ["Status", <StatusBadge value={w.status} />],
          ["Application", <AppName id={w.application_id} />],
          ["Workflow instance ID", <IdChip value={w.id} />],
        ]}
      />
      <button type="button" className="btn btn-sm" onClick={open}>Open in Workflows</button>
    </>
  );
}

function StartDetail({ s, at, tz }: { s: StartSchedule; at: Date; tz: string }) {
  return (
    <>
      <Fields
        rows={[
          ["This run", fmt(at.toISOString(), tz)],
          ["Stored next run", fmt(s.next_at, tz)],
          ["Application", <AppName id={s.application_id} />],
        ]}
      />
      <p className="text-xs text-base-content/50">Changes apply to every future run of this Start schedule.</p>
      <StartScheduleForm key={s.id + s.updated_at} appId={s.application_id} initial={s} onSaved={() => undefined} />
    </>
  );
}
