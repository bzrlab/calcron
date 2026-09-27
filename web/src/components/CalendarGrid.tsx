import { useEffect, useMemo, useRef, useState, type CSSProperties, type DragEvent, type ReactNode } from "react";
import { dayOpen, days, fromYmd, hhmm, moveIntent, shift, ymd, zoned, type CalendarDefinition, type CalendarMoveIntent, type Mode } from "../lib/calendar";

export type Tone = "info" | "warning" | "error" | "success" | "primary" | "neutral";
export type GridEvent = { key: string; at: Date; label: string; tone: Tone; movable?: boolean };
type Placed = GridEvent & { minutes: number };

const TONE: Record<Tone, string> = {
  info: "border-info bg-info/15",
  warning: "border-warning bg-warning/15",
  error: "border-error bg-error/20",
  success: "border-success bg-success/15",
  primary: "border-primary bg-primary/15",
  neutral: "border-base-content/30 bg-base-content/5 text-base-content/60",
};
const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MODES: Mode[] = ["month", "week", "day"];
const HOUR = 48;
const SNAP = 15;
const HATCH: CSSProperties = {
  backgroundImage:
    "repeating-linear-gradient(135deg, transparent 0 6px, color-mix(in oklab, var(--color-base-content) 8%, transparent) 6px 7px)",
};
const hatch = (state: { open: boolean } | null) => (state && !state.open ? HATCH : undefined);

// Browsers hide drag data until drop, so the live drop preview reads the chip being dragged from here.
// Only the time of day moves; a drop is accepted on the chip's own day.
let dragging: { key: string; day: string; grab: number } | null = null;

const zoneName = (timeZone: string) =>
  new Intl.DateTimeFormat(undefined, { timeZone, timeZoneName: "short" }).formatToParts(new Date()).find((x) => x.type === "timeZoneName")?.value;

type Props = {
  mode: Mode;
  anchor: Date;
  onMode: (m: Mode) => void;
  onAnchor: (d: Date) => void;
  timeZone: string;
  events?: GridEvent[];
  selectedKey?: string;
  onEvent?: (key: string) => void;
  onMove?: (intent: CalendarMoveIntent) => void;
  definition?: CalendarDefinition | null;
  onDay?: (day: string) => void;
  onWeekday?: (weekday: number) => void;
  toolbar?: ReactNode;
};

export function CalendarGrid(p: Props) {
  const shown = days(p.mode, p.anchor);
  const byDay = useMemo(() => {
    const m = new Map<string, Placed[]>();
    for (const e of p.events ?? []) {
      const z = zoned(e.at, p.timeZone);
      m.set(z.day, [...(m.get(z.day) ?? []), { ...e, minutes: z.minutes }]);
    }
    for (const list of m.values()) list.sort((a, b) => a.at.getTime() - b.at.getTime());
    return m;
  }, [p.events, p.timeZone]);

  const title =
    p.mode === "month"
      ? p.anchor.toLocaleDateString(undefined, { month: "long", year: "numeric" })
      : p.mode === "week"
        ? `${shown[0].toLocaleDateString(undefined, { month: "short", day: "numeric" })} – ${shown[6].toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}`
        : p.anchor.toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric", year: "numeric" });

  const openDay = (d: Date) => {
    p.onAnchor(d);
    p.onMode("day");
  };

  return (
    <div className="rounded-box border border-base-300 bg-base-100">
      <div className="flex flex-wrap items-center gap-2 border-b border-base-300 px-4 py-3">
        <button type="button" className="btn btn-outline btn-sm" onClick={() => p.onAnchor(fromYmd(zoned(new Date(), p.timeZone).day))}>Today</button>
        <div className="join">
          <button type="button" className="btn btn-ghost btn-sm join-item" aria-label="Previous" onClick={() => p.onAnchor(shift(p.mode, p.anchor, -1))}>‹</button>
          <button type="button" className="btn btn-ghost btn-sm join-item" aria-label="Next" onClick={() => p.onAnchor(shift(p.mode, p.anchor, 1))}>›</button>
        </div>
        <h3 className="font-semibold" aria-live="polite">{title}</h3>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {p.toolbar}
          <div className="join" role="group" aria-label="Calendar layout">
            {MODES.map((m) => (
              <button
                key={m}
                type="button"
                aria-pressed={p.mode === m}
                className={`btn btn-sm join-item capitalize ${p.mode === m ? "btn-primary" : "btn-ghost"}`}
                onClick={() => p.onMode(m)}
              >
                {m}
              </button>
            ))}
          </div>
        </div>
      </div>
      {p.mode === "month" ? (
        <MonthGrid p={p} shown={shown} byDay={byDay} openDay={openDay} />
      ) : (
        <HourGrid p={p} shown={shown} byDay={byDay} />
      )}
    </div>
  );
}

function Chip({ e, p, dragDay }: { e: GridEvent; p: Props; dragDay?: string }) {
  const drag = !!dragDay;
  const at = e.at.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", timeZone: p.timeZone });
  return (
    <button
      type="button"
      title={`${at} ${e.label}${drag ? " (drag up or down to change the time)" : ""}`}
      draggable={drag}
      onDragStart={(ev) => {
        dragging = { key: e.key, day: dragDay!, grab: ev.clientY - ev.currentTarget.getBoundingClientRect().top };
        ev.dataTransfer.setData("text/plain", e.label);
        ev.dataTransfer.effectAllowed = "move";
      }}
      onDragEnd={() => { dragging = null; }}
      onClick={(ev) => { ev.stopPropagation(); p.onEvent?.(e.key); }}
      className={`block h-full w-full truncate rounded border-l-2 px-1.5 py-0.5 text-left text-xs transition hover:brightness-95 ${TONE[e.tone]} ${drag ? "cursor-grab active:cursor-grabbing" : ""} ${p.selectedKey === e.key ? "ring-2 ring-base-content" : ""}`}
    >
      <span className="tabular-nums opacity-70">{at}</span> {e.label}
    </button>
  );
}

function WeekdayHeader({ p, weekday }: { p: Props; weekday: number }) {
  const name = WEEKDAYS[weekday];
  if (!p.onWeekday) return <div className="py-2 text-center">{name}</div>;
  const on = p.definition?.weekdays.includes(weekday);
  return (
    <button
      type="button"
      className={`py-2 hover:bg-base-200 ${on ? "" : "line-through opacity-60"}`}
      aria-pressed={on}
      title={`${on ? "Close" : "Open"} every ${name}`}
      onClick={() => p.onWeekday!(weekday)}
    >
      {name}
    </button>
  );
}

function MonthGrid({ p, shown, byDay, openDay }: { p: Props; shown: Date[]; byDay: Map<string, Placed[]>; openDay: (d: Date) => void }) {
  const today = zoned(new Date(), p.timeZone).day;
  return (
    <div>
      <div className="grid grid-cols-7 border-b border-base-300 text-xs text-base-content/60">
        {WEEKDAYS.map((_, i) => <WeekdayHeader key={i} p={p} weekday={i} />)}
      </div>
      <div className="grid grid-cols-7">
        {shown.map((d) => {
          const day = ymd(d);
          const list = byDay.get(day) ?? [];
          const state = p.definition ? dayOpen(p.definition, day) : null;
          const outside = d.getMonth() !== p.anchor.getMonth();
          const cell = `relative min-h-24 border-b border-r border-base-300 p-1.5 text-left ${outside ? "text-base-content/35" : ""}`;
          const number = (
            <span className={`grid h-6 min-w-6 place-items-center rounded-full px-1 text-xs ${day === today ? "bg-primary text-primary-content" : ""}`}>
              {d.getDate()}
            </span>
          );
          const badge = state?.override && (
            <span className={`badge badge-xs ${state.open ? "badge-success" : "badge-error"}`}>{state.open ? "open" : "closed"}</span>
          );
          const chips = (
            <div className="mt-1 space-y-0.5">
              {list.slice(0, 3).map((e) => <Chip key={e.key} e={e} p={p} />)}
              {list.length > 3 && (
                <span role="button" tabIndex={0} className="text-xs text-base-content/60 hover:underline" onClick={(ev) => { ev.stopPropagation(); openDay(d); }} onKeyDown={(ev) => { if (ev.key === "Enter") { ev.stopPropagation(); openDay(d); } }}>
                  +{list.length - 3} more
                </span>
              )}
            </div>
          );
          if (p.onDay) {
            return (
              <div
                key={day}
                role="button"
                tabIndex={0}
                className={`${cell} cursor-pointer hover:bg-base-200`}
                style={hatch(state)}
                aria-label={`${d.toDateString()}: ${state?.open ? "open" : "closed"}${state?.override ? " (override)" : ""}. Toggle for this date.`}
                onClick={() => p.onDay!(day)}
                onKeyDown={(ev) => {
                  if (ev.target !== ev.currentTarget || (ev.key !== "Enter" && ev.key !== " ")) return;
                  ev.preventDefault();
                  p.onDay!(day);
                }}
              >
                <div className="flex items-center justify-between">{number}{badge}</div>
                {chips}
              </div>
            );
          }
          return (
            <div key={day} className={cell} style={hatch(state)}>
              <div className="flex items-center justify-between">
                <button type="button" className="rounded-full hover:bg-base-200" aria-label={`Open ${d.toDateString()}`} onClick={() => openDay(d)}>{number}</button>
                {badge}
              </div>
              {chips}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// Items are instants, not durations: each gets a half-hour slot, and items sharing a slot split its width.
function place(list: Placed[]) {
  const slots = new Map<number, Placed[]>();
  for (const e of list) {
    const s = Math.floor(e.minutes / 30);
    slots.set(s, [...(slots.get(s) ?? []), e]);
  }
  return [...slots.values()].flatMap((group) => group.map((e, col) => ({ e, col, cols: group.length })));
}

function HourGrid({ p, shown, byDay }: { p: Props; shown: Date[]; byDay: Map<string, Placed[]> }) {
  const scroller = useRef<HTMLDivElement>(null);
  const [now, setNow] = useState(() => new Date());
  const [hover, setHover] = useState<{ day: string; minutes: number } | null>(null);
  const current = zoned(now, p.timeZone);
  const showsToday = shown.some((d) => ymd(d) === current.day);
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 60_000);
    return () => clearInterval(t);
  }, []);
  useEffect(() => {
    const hour = Math.floor(zoned(new Date(), p.timeZone).minutes / 60);
    scroller.current?.scrollTo({ top: Math.max(0, (showsToday ? hour - 2 : 7) * HOUR) });
  }, [p.mode, p.timeZone, showsToday]);
  const cols = { gridTemplateColumns: `3.5rem repeat(${shown.length}, minmax(0, 1fr))` };

  const slotAt = (ev: DragEvent<HTMLDivElement>, grab: number) => {
    const y = ev.clientY - ev.currentTarget.getBoundingClientRect().top - grab;
    return Math.min(24 * 60 - SNAP, Math.max(0, Math.round((y / HOUR) * 60 / SNAP) * SNAP));
  };
  const drop = p.onMove && ((day: string) => ({
    onDragOver: (ev: DragEvent<HTMLDivElement>) => {
      if (dragging?.day !== day) return;
      ev.preventDefault();
      const minutes = slotAt(ev, dragging.grab);
      if (hover?.day !== day || hover.minutes !== minutes) setHover({ day, minutes });
    },
    onDragLeave: () => setHover(null),
    onDrop: (ev: DragEvent<HTMLDivElement>) => {
      if (dragging?.day !== day) return;
      ev.preventDefault();
      setHover(null);
      p.onMove!(moveIntent(dragging.key, day, slotAt(ev, dragging.grab), p.timeZone));
      dragging = null;
    },
  }));

  return (
    <div>
      <div className="grid overflow-hidden border-b border-base-300" style={{ ...cols, scrollbarGutter: "stable" }}>
        <div className="self-end pb-2 text-center text-[0.65rem] text-base-content/40" title={`Times shown in ${p.timeZone}`}>{zoneName(p.timeZone)}</div>
        {shown.map((d) => {
          const day = ymd(d);
          const state = p.definition ? dayOpen(p.definition, day) : null;
          const body = (
            <>
              <div className="text-xs text-base-content/60">{WEEKDAYS[d.getDay()]}</div>
              <div className={`text-lg font-semibold leading-tight ${day === current.day ? "text-primary" : ""}`}>{d.getDate()}</div>
              {state && <div className="text-[0.65rem] text-base-content/50">{state.open ? "open" : "closed"}{state.override ? " · override" : ""}</div>}
            </>
          );
          return p.onDay ? (
            <button key={day} type="button" className="border-l border-base-300 px-2 py-2 text-left hover:bg-base-200" style={hatch(state)} onClick={() => p.onDay!(day)}>
              {body}
            </button>
          ) : (
            <div key={day} className="border-l border-base-300 px-2 py-2" style={hatch(state)}>{body}</div>
          );
        })}
      </div>
      <div ref={scroller} className="max-h-[max(34rem,calc(100vh-20rem))] overflow-y-auto" style={{ scrollbarGutter: "stable" }}>
        <div className="relative grid" style={{ ...cols, height: HOUR * 24 }}>
          <div>
            {Array.from({ length: 24 }, (_, h) => (
              <div key={h} className="pr-2 text-right text-[0.65rem] text-base-content/40" style={{ height: HOUR }}>
                {h > 0 && <span className="relative -top-2">{new Date(2000, 0, 1, h).toLocaleTimeString(undefined, { hour: "numeric" })}</span>}
              </div>
            ))}
          </div>
          {shown.map((d) => {
            const day = ymd(d);
            const state = p.definition ? dayOpen(p.definition, day) : null;
            return (
              <div key={day} className="relative border-l border-base-300" style={hatch(state)} {...drop?.(day)}>
                {Array.from({ length: 24 }, (_, h) => <div key={h} className="border-b border-base-300/60" style={{ height: HOUR }} />)}
                {place(byDay.get(day) ?? []).map(({ e, col, cols: n }) => (
                  <div
                    key={e.key}
                    className="absolute px-0.5"
                    style={{ top: (e.minutes / 60) * HOUR, height: HOUR / 2, left: `${(col / n) * 100}%`, width: `${100 / n}%` }}
                  >
                    <Chip e={e} p={p} dragDay={p.onMove && e.movable ? day : undefined} />
                  </div>
                ))}
                {hover?.day === day && (
                  <div className="pointer-events-none absolute inset-x-0 z-20 border-t-2 border-dashed border-primary" style={{ top: (hover.minutes / 60) * HOUR }}>
                    <span className="absolute left-1 -top-5 rounded bg-primary px-1 text-[0.65rem] text-primary-content">{hhmm(hover.minutes)}</span>
                  </div>
                )}
                {day === current.day && (
                  <div className="pointer-events-none absolute inset-x-0 z-10 border-t-2 border-error" style={{ top: (current.minutes / 60) * HOUR }}>
                    <span className="absolute -left-1.5 -top-1.5 h-3 w-3 rounded-full bg-error" />
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
