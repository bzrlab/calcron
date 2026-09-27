export type Mode = "month" | "week" | "day";
export type CalendarDefinition = { timezone: string; weekdays: number[]; overrides?: Record<string, boolean> };
export type CalendarWallTime = Readonly<{ day: string; minutes: number; timeZone: string }>;
export type CalendarMoveIntent = Readonly<{ type: "calendar.move"; eventKey: string; display: CalendarWallTime }>;

export const BROWSER_ZONE = Intl.DateTimeFormat().resolvedOptions().timeZone;
export const ZONES = Intl.supportedValuesOf("timeZone");

const pad = (n: number) => String(n).padStart(2, "0");

export const ymd = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

function addDays(d: Date, n: number) {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}

export function days(mode: Mode, anchor: Date): Date[] {
  if (mode === "day") return [addDays(anchor, 0)];
  const first = mode === "month" ? new Date(anchor.getFullYear(), anchor.getMonth(), 1) : anchor;
  const start = addDays(first, -first.getDay());
  return Array.from({ length: mode === "month" ? 42 : 7 }, (_, i) => addDays(start, i));
}

export function shift(mode: Mode, anchor: Date, dir: 1 | -1): Date {
  if (mode === "month") return new Date(anchor.getFullYear(), anchor.getMonth() + dir, 1);
  return addDays(anchor, dir * (mode === "week" ? 7 : 1));
}

export function fromYmd(day: string) {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, m - 1, d);
}

const weekday = (day: string) => fromYmd(day).getDay();

export const hhmm = (minutes: number) => `${pad(Math.floor(minutes / 60))}:${pad(minutes % 60)}`;

const formatters = new Map<string, Intl.DateTimeFormat>();
function parts(d: Date, timeZone: string) {
  let f = formatters.get(timeZone);
  if (!f) {
    f = new Intl.DateTimeFormat("en-US", { timeZone, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
    formatters.set(timeZone, f);
  }
  const p = Object.fromEntries(f.formatToParts(d).map((x) => [x.type, Number(x.value)]));
  return { y: p.year, m: p.month, d: p.day, h: p.hour, min: p.minute };
}

export function zoned(d: Date, timeZone: string) {
  const p = parts(d, timeZone);
  return { day: `${p.y}-${pad(p.m)}-${pad(p.d)}`, minutes: p.h * 60 + p.min };
}

// wallTime is the instant a timezone's clock shows `minutes` past midnight on `day`.
export function wallTime(day: string, minutes: number, timeZone: string): Date {
  const [y, m, d] = day.split("-").map(Number);
  const guess = Date.UTC(y, m - 1, d, 0, minutes);
  const offset = (t: number) => {
    const p = parts(new Date(t), timeZone);
    return Date.UTC(p.y, p.m - 1, p.d, p.h, p.min) - Math.floor(t / 60_000) * 60_000;
  };
  const first = guess - offset(guess);
  return new Date(guess - offset(first));
}

// A grid edit is a wall-clock instruction in the display zone, not an unqualified instant.
// Callers can explicitly project it into the calendar whose rule they are changing.
export function moveIntent(eventKey: string, day: string, minutes: number, timeZone: string): CalendarMoveIntent {
  if (!eventKey) throw new RangeError("A calendar move needs an event key");
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day) || ymd(fromYmd(day)) !== day) throw new RangeError(`Invalid calendar day: ${day}`);
  if (!Number.isInteger(minutes) || minutes < 0 || minutes >= 24 * 60) throw new RangeError(`Invalid calendar minute: ${minutes}`);
  // Constructing a formatter makes an invalid IANA zone fail at the module seam.
  new Intl.DateTimeFormat("en-US", { timeZone });
  return { type: "calendar.move", eventKey, display: { day, minutes, timeZone } };
}

export function projectMoveIntent(intent: CalendarMoveIntent, timeZone: string): CalendarWallTime {
  const { display } = intent;
  if (display.timeZone === timeZone) return display;
  return { ...zoned(wallTime(display.day, display.minutes, display.timeZone), timeZone), timeZone };
}

export function dayOpen(def: CalendarDefinition, day: string) {
  const o = def.overrides?.[day];
  return o === undefined ? { open: def.weekdays.includes(weekday(day)), override: false } : { open: o, override: true };
}

export function cycleOverride(def: CalendarDefinition, day: string): Record<string, boolean> {
  const { [day]: existing, ...rest } = def.overrides ?? {};
  return existing === undefined ? { ...rest, [day]: !def.weekdays.includes(weekday(day)) } : rest;
}
