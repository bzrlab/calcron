export const DASHBOARD_LISTS = [
  "apps",
  "schedules",
  "deliveries",
  "workflows",
  "history",
  "calendars",
] as const;

export type DashboardList = (typeof DASHBOARD_LISTS)[number];
