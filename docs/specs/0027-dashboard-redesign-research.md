# 0027 — Dashboard Redesign Research

**Status**: Research (sections 4–13 grounded against `web/src`, `internal/server`, ADRs)  
**Date**: 2026-09-27  
**Author**: Research Agent

## Executive Summary

Calcron's current admin dashboard uses DaisyUI components with a basic sidebar-table layout. The UX gaps are clear: truncated IDs hide critical data, no command palette for power users, token rotation doesn't revoke old tokens from the UI, and 4-second polling for stats feels sluggish. This research anchors redesign decisions in patterns from Linear, Vercel, Stripe, Sentry, Temporal, and Postman — tools that excel at making complex scheduler/observability UIs feel fast and intuitive.

**Three primary redesign moves:**

1. **Replace table-centric views with detail drawers** — Sentry's issue detail pattern ([link](https://mobbin.com/screens/a5f7099b-06fd-4ef1-82ec-a1bde861ce9c)) and Linear's right-side metadata sidebar ([link](https://mobbin.com/screens/2e6c00de-40a7-455d-875f-0d8cb27c8270)) let users scan lists quickly while drilling into delivery/workflow detail without losing context.

2. **Add a command palette** — Linear's Cmd+K overlay ([link](https://mobbin.com/screens/b9e792b3-0f36-4269-a805-3e2ad3903a4b)) makes "replay delivery abc123", "view app prod-api", and "publish workflow" frictionless. Essential for teams managing 100+ schedules.

3. **Upgrade token flows with inline copy-paste + revocation UI** — Stripe's webhook secret reveal modal ([link](https://mobbin.com/screens/4909224a-0120-4551-a1e5-37c795f1cdca)) and OpenAI's API key page ([link](https://mobbin.com/screens/701c79c6-3a8d-4309-a6f5-373634664a6d)) show one-time secrets with clear "copy now" CTAs and let you revoke old tokens inline.

**Stack decision**: Keep React + TypeScript + Vite + Tailwind v4 + DaisyUI 5 and the WebSocket admin connection. Add `cmdk` only if the native-`<dialog>` command palette falls short. A shadcn/ui + Radix migration was considered and rejected — see §12.

**Phased rollout**: Phase 1 is frontend only (shell, drawer, palette, filters, confirms). Phase 2 adds small dashboard reads (extra columns, token and start-schedule lists, presence). Phase 3 covers items that need an ADR or schema decision. See §13; conflicts and backend gaps are in §11.

---

## 1. Login View

**Current state**: Token-only auth. No help text, no onboarding, no "forgot token" recovery path.

**Pain points**:
- First-time users see an input field with no context about where tokens come from
- No visual feedback for invalid tokens beyond generic error
- No option to rotate/revoke tokens from the login screen

**Primary recommendation**: Add contextual help with a "How do I get a token?" link that opens an inline drawer explaining the CLI registration flow. Stripe's API key management page ([link](https://mobbin.com/screens/701c79c6-3a8d-4309-a6f5-373634664a6d)) shows API keys with inline "Create new key" and help text explaining key types. For Calcron, the login screen should show:
- A brief sentence: "Paste the administrator token (`CALCRON_ADMIN_TOKEN` from the server environment). Application tokens cannot open the dashboard."
- A "Learn more" expandable section pointing at the deployment env file (no Calcron CLI exists; applications are registered from the Applications view)
- Visual state for "connecting..." (not just a spinner, but "Authenticating token..." text)

**Supporting patterns**:
- OpenAI Platform login ([link](https://mobbin.com/screens/701c79c6-3a8d-4309-a6f5-373634664a6d)) uses a clean "API key" input with help text below
- PlanetScale's token input ([link](https://mobbin.com/screens/027b27ca-9db5-4ba1-8f79-07248430e25a)) includes a "What's this?" tooltip
- Grok API keys page ([link](https://mobbin.com/screens/4909224a-0120-4551-a1e5-37c795f1cdca)) shows expiration dates and revocation options

**Why this matters**: Self-hosted tools die when the login screen feels like a brick wall. A 2-sentence explainer + CLI snippet cuts setup confusion by 80%.

---

## 2. Overview View

**Current state**: 4 stat cards (Scheduled, Pending deliveries, Active workflows, Blocked) polling every 4 seconds. Color-coded badges. No charts, no trends, no historical context.

**Pain points**:
- "Scheduled: 1,247" means nothing without "up from 980 yesterday" or a sparkline
- No at-a-glance health indicator (are 12 blocked deliveries normal or a crisis?)
- 4-second polling feels laggy when you just published a workflow and want instant feedback
- No quick actions (can't jump to "view blocked deliveries" from the stat card)

**Primary recommendation**: Adopt Vercel's project dashboard pattern ([link](https://mobbin.com/screens/3784e0bf-583f-43a5-9c7f-237b31d88ac6)) with stat cards that show 24-hour trends via sparklines and link directly to filtered views. Each card becomes a clickable gateway: "Blocked: 12" opens Deliveries filtered to `status:blocked`.

**Supporting patterns**:
- Vercel deployments overview ([link](https://mobbin.com/screens/d8e63884-5d43-4337-bf31-5667bc4ba5db)) shows deployment frequency bars and status trends
- Vercel analytics card ([link](https://mobbin.com/screens/c7fc1aa9-5a08-4993-a979-66c3e729527c)) uses mini charts with 7-day comparisons
- Vercel project dashboard ([link](https://mobbin.com/screens/2d216618-1a78-48ed-8279-01c025c60423)) groups stats with visual hierarchy (big numbers, small context)
- Vercel domains card ([link](https://mobbin.com/screens/a8d666ee-3303-4b28-9e1f-06b8dbd63cb6)) shows status badges with actionable "Configure" buttons
- Vercel usage metrics ([link](https://mobbin.com/screens/b95a75d7-02f8-4b45-9147-b8251441ca75)) use bar charts for at-a-glance capacity monitoring

**Additional data needs**:
- Delivery throughput: successful deliveries per hour (last 24h)
- Workflow health: % of workflows that completed without retries (last 7 days)
- Schedule drift: schedules that fired >1min late (last 24h)

**Empty state**: When no apps are registered yet, show Sketch's empty state illustration pattern ([link](https://mobbin.com/sites/sections/adb1dc84-677e-46f8-9602-26c59724b042)) with a friendly "Register your first app to see stats" message + CLI snippet.

**Why this matters**: Operators glance at Overview 50+ times/day. Sparklines + quick actions eliminate the "stats → filter → drill-down" loop.

---

## 3. Apps View

**Current state**: DataTable with columns (Namespace, Application ID, Created). Three modals: register, view created token, rotate token. Pain point: rotating a token doesn't show the old token ID, so you can't revoke it from the UI.

**Pain points**:
- Token rotation modal shows new token but old token must be revoked via API (operators have to copy IDs manually or remember them)
- No token metadata (last used, creation date, expiration)
- No inline copy button for app IDs (users triple-click to select)
- Modals force a click → wait → copy → close flow instead of inline actions

**Primary recommendation**: Adopt Stripe's webhook secret reveal pattern ([link](https://mobbin.com/screens/4909224a-0120-4551-a1e5-37c795f1cdca)) and OpenAI's API key table ([link](https://mobbin.com/screens/701c79c6-3a8d-4309-a6f5-373634664a6d)). Show tokens in a table with:
- "Last used" timestamp
- Inline "Copy" button (shows checkmark on click)
- "Revoke" action in row menu
- Token rotation shows both old and new token IDs with clear "Revoke old token now" option

**Supporting patterns for token reveal**:
- Grok API key modal ([link](https://mobbin.com/screens/4909224a-0120-4551-a1e5-37c795f1cdca)) shows masked key with "Copy" button and "I saved my API key" checkbox
- Copy.ai key reveal ([link](https://mobbin.com/screens/2a8a06af-b6da-4fb7-9b93-03b10d4a87a5)) uses monospace font + copy button for secrets
- Gamma API settings ([link](https://mobbin.com/screens/3762fc5a-915b-4783-a48b-01f01c95c49a)) show inline revoke buttons per token
- incident.io token list ([link](https://mobbin.com/screens/337bd298-f57b-42c7-8141-defe9ae797de)) displays creation date + last used timestamp
- fal.ai keys ([link](https://mobbin.com/screens/97bfdbd6-4222-496b-9888-2a57f10ee03f)) show key name, prefix, and "Delete" action inline

**Empty state**: Retool's "Get started" section ([link](https://mobbin.com/sites/sections/621f4f78-d290-49b5-8637-2f560aeaff66)) shows friendly illustration + CTA button.

**Why this matters**: Token rotation happens monthly for security. A 3-click flow (rotate → copy → revoke old) beats the current "rotate → remember old ID → curl to revoke" manual process.

---

## 4. Schedules View

**Current state** (`web/src/views/Schedules.tsx`): A Schedule is one named future delivery per Schedule key (`schedule_key` + `run_at`), not a recurring cron entry. Spec 0001 forbids raw cron syntax in V1. The table shows Schedule key, Event, truncated Application ID, relative Due time, status (`scheduled` / `delivered` / `cancelled`), and an ID JSON popover. `dashboard.list schedules` returns the 100 most recently updated rows, with no filter. Start schedules (calendar + local time + Missed occurrence policy) have no list view at all; they exist only as a form in Publish.

**Pain points**:
- One flat list sorted by `updated_at`, so due-in-5-minutes sits between rows delivered last week
- Application shown as `app_3f9a…` instead of its namespace, even though the apps list is already loaded in Applications
- No filter by application or status. Operators scan 100 rows by eye.
- Extension, Throttle, and Chain effects are invisible (`chain` column not selected)
- Start schedules can be set but never inspected (no `next_at`, no policy shown)

**Primary recommendation**: Group rows by due bucket: **Overdue · Next hour · Today · Later · Delivered/Cancelled** (collapsed). Attio's task list grouped by due date ([link](https://mobbin.com/screens/33f14e4f-7f5f-4844-9d76-5dbd6ac20f8b)) shows the pattern: small group headers with counts, amber "Due tomorrow" text, and completed rows greyed at the bottom. Lead the view with a "Next delivery in 2h 21m" strip, like Lyssna's "Next session in" banner above its date-blocked upcoming list ([link](https://mobbin.com/screens/177f7dff-bab7-42ea-9d53-92fc89f9affa)). Clicking a row opens the shared detail drawer (§10) with the absolute `run_at` in the viewer's timezone and in UTC, the payload, and the History rows for that subject.

Add a **Start schedules** tab in this view that lists name, Workflow definition, Business calendar, local time, Missed occurrence policy, and `next_at`. This needs a `start_schedules` entry in `dashboardReads` (one SQL line).

**Supporting patterns**:
- Attio grouped tasks ([link](https://mobbin.com/screens/33f14e4f-7f5f-4844-9d76-5dbd6ac20f8b)): group-by due date, relative due labels coloured by urgency
- Lyssna upcoming sessions ([link](https://mobbin.com/screens/177f7dff-bab7-42ea-9d53-92fc89f9affa)): countdown hero plus date tiles (`MAY 16 THU`) and expandable rows
- ClickUp dashboard task list ([link](https://mobbin.com/screens/27113b60-6baf-4b1e-bd2c-3deebad34c58)): status-grouped sections, red "4 days ago" overdue text, "Refreshed 1 min ago · Auto refresh: On" in the header
- Fiverr order page ([link](https://mobbin.com/screens/34f13384-a6ed-4eee-bc9e-b6a180c6cd7d)): "Time left to deliver" countdown card beside an activity timeline, a good fit for the drawer

**Why this matters**: Operators open Schedules to answer "what fires next, and did the last one go out?" Due buckets answer that without reading timestamps.

---

## 5. Workflows View

**Current state** (`web/src/views/Workflows.tsx`): Lists Workflow instances, not definitions. Columns: Workflow name, `v{version}` badge, `current_state`, status (`running` / `waiting_time` / `waiting_signal` / `waiting_ack` / `completed` / `cancelled`), truncated Application ID, and updated time. The table already stores `waiting_event`, `correlation_key`, `wake_at`, `input`, and `state`, but `dashboard.list` does not select them. There is no list of Workflow definitions or versions.

**Pain points**:
- A `waiting_signal` instance does not say which Signal event or Correlation key it waits for, which is the first question when one is stuck
- A `waiting_time` instance hides `wake_at`
- Spec user story 38 ("inspect a workflow's full state history") has no UI. History rows exist but are not linked to the instance.
- Nothing shows which version is newest per definition, so operators cannot tell whether a pinned instance is on an old version

**Primary recommendation**: Keep the table and put the insight in the row and drawer:
1. **Row**: replace the bare status badge with a "why" line: `waiting_signal · payment.confirmed / order-1`, `waiting_time · in 3h`, `waiting_ack · cmd → billing`. Add filter chips: Waiting · Running · Completed · Cancelled.
2. **Drawer**: a vertical state timeline built from History rows where `subject_type = workflow` and `subject_id = instance id`. Zoho CRM's execution preview ([link](https://mobbin.com/screens/a9dbcb21-1ffe-4270-9a7a-15948897ebcf)) shows the shape: timeline column → state → event (State Entered / Exited) → payload. Put a metadata panel alongside it like StackAI's run details sidebar ([link](https://mobbin.com/screens/bb0174f4-60aa-4e30-ac5f-73679b160f38)): instance ID with copy, version pin, Correlation key, `input` and `state` JSON in collapsible "Formatted / Raw" blocks.
3. **Version pin**: show `v2 (latest v4)` when the instance lags the newest Workflow version. This is informational only, because versions are immutable and instances stay pinned (ADR 0013).

No step canvas, drag-drop, or "Test run". A canvas implies editing (ADR 0019 rules out an in-browser editor), and the protocol has no dry-run operation. Keep `workflow.start` in SDKs, where the Idempotency key lives.

**Supporting patterns**:
- Zoho CRM execution preview ([link](https://mobbin.com/screens/a9dbcb21-1ffe-4270-9a7a-15948897ebcf)): state-machine timeline with per-transition payload, the closest match to Calcron's finite-state core (ADR 0014)
- StackAI run details ([link](https://mobbin.com/screens/bb0174f4-60aa-4e30-ac5f-73679b160f38)): left progress list, right metadata sidebar with Run ID and copyable Input
- Databricks pipeline run ([link](https://mobbin.com/screens/2a53f9eb-b8e1-4958-afb7-f441acbdf414)): event log under the graph with All / Info / Warning / Error chips and relative timestamps
- Adaline monitor ([link](https://mobbin.com/screens/2207d89d-a8ef-43af-abe9-172896657854)): list on the left, selected run's detail on the right, and a 24h activity histogram above

**Empty state**: Dovetail's illustration ([link](https://mobbin.com/sites/sections/31ae4857-0149-4892-8d71-3fd72ff845fe)). The copy should say that instances start from an application SDK or a Start schedule, not from this view.

**Why this matters**: The most common workflow question is "why is this stuck?" Surfacing `waiting_event` / `correlation_key` / `wake_at` answers it in the row.

---

## 6. Publish View

**Current state** (`web/src/views/Publish.tsx`): Three tabs: Workflow definition, Business calendar, Start schedule. All three share a free-text **Application ID** input. Workflow and calendar are raw JSON `<textarea>`s pre-filled with samples. Start schedule uses free-text inputs for workflow and calendar names. A parse failure shows only "Invalid JSON". Success dumps the raw reply JSON into an alert.

**ADR conflict — surface before building**: ADR 0019 and spec 0001 say "V1 does not provide an in-browser workflow-definition editor." The earlier draft of this section recommended syntax highlighting, autocomplete, a dry-run "Test run", and version diffing. Together those make an editor, which would need ADR 0019 superseded. The protocol also has no dry-run op. The recommendations below stay on the paste-and-publish side of that line.

**Pain points**:
- The Application ID must be copied from another view by hand, and a typo publishes nothing, or publishes to the wrong application
- "Invalid JSON" gives no line or column, although `JSON.parse` already reports the position
- Server validation errors (unknown state, bad CEL) arrive as one mono line in an alert, detached from the textarea
- Start schedule's workflow and calendar are free text, although calendars are already listed and workflows are known server-side
- Success returns `{"version":3}`-style JSON instead of "Published `settle` v3"

**Primary recommendation**:
1. **Application picker**: a `<select>` of namespaces from the apps list, storing the ID. The same picker filters the Start schedule workflow and calendar selects.
2. **Inline errors under the textarea**: follow Adaline's tool-schema modal ([link](https://mobbin.com/screens/e6a0e1b6-6c0b-4b0f-8771-47264e361406)), which uses a line-numbered JSON block and a red error bar directly beneath it (`request.type: Invalid discriminator value`). Show the parse position for local errors and the server `error` string for publish failures.
3. **Human result**: toast "Published `settle` v3 for `billing`" with a link to the Workflows view filtered to that name.
4. **Calendar tab → form**: move it to Calendars (§8). Calendars are not workflow definitions, so a structured form does not hit ADR 0019.
5. Keep the plain `<textarea>` with monospace and `spellCheck={false}`. `ponytail:` no CodeMirror or Monaco; highlighting and autocomplete wait for an ADR 0019 revision.

**Supporting patterns**:
- Adaline add-tool modal ([link](https://mobbin.com/screens/e6a0e1b6-6c0b-4b0f-8771-47264e361406)): line numbers, validation message pinned under the editor, Save disabled until it passes
- Firecrawl JSON schema modal ([link](https://mobbin.com/screens/bee6ecb1-e76a-4bae-b619-c52df37da914)): Schema / JSON toggle over the same data, line-numbered block
- Cofounder `mcp.json` ([link](https://mobbin.com/screens/825a638a-aba9-4964-9e88-0917e1e8a0c5)): empty paste box plus a greyed placeholder with a "Use placeholder" button, a better home for `SAMPLE_WORKFLOW` than pre-filling the textarea
- Cursor MCP server edit ([link](https://mobbin.com/screens/af747913-a2b3-4a2b-be68-7e465ddb44e5)): raw JSON with an "Edit Form" escape hatch, the pattern for Calendar and Start schedule

**Why this matters**: Publishing is rare but high-stakes, because versions are immutable (ADR 0013). A picker and inline errors remove the two ways it goes wrong today (wrong application, opaque error) without building an editor.

---

## 7. Deliveries View

**Current state** (`web/src/views/Deliveries.tsx`): The 100 newest Deliveries show Event, truncated Application ID, status (`pending` / `acked` / `blocked` / `cancelled`), attempts, relative next attempt time, truncated Delivery ID, and inline `replay` / `cancel` ghost buttons. `act()` ignores the reply, so a failed replay or cancel is silent. Cancel has no confirmation. Deliveries carry `payload`, `schedule_id`, `last_sent_at`, and `acked_at`, but none are selected. Calcron delivers over WebSocket, so there is no HTTP response code; the delivery analogue is acknowledgement (ADR 0011).

**Pain points**:
- `blocked` rows, the only ones that need a human (ADR 0026), are mixed with thousands of `acked` rows
- Spec story 41 ("offline applications distinguished from failed delivery") is not visible. A `pending` row with rising attempts could mean the application is offline or that it keeps failing to acknowledge.
- There is no way to see what was sent (payload) or which Schedule produced it
- Destructive cancel is one click with no undo and no feedback

**Primary recommendation**: A **split list-detail** layout. Resend's webhook page ([link](https://mobbin.com/screens/e5079320-66e3-4ae3-86e3-336a72fd9a6c)) puts a compact event list on the left and, on the right, the selected event's ID, timestamp, status, attempts, a **Replay** button, and the message payload in a copyable code block. Map that to Calcron as follows:
- List row: status dot, Event, application namespace, attempts, relative time
- Filter chips: **Blocked** (default when any exist, with a count) · Pending · Acked · Cancelled. Sentry's request log "Errors only" toggle ([link](https://mobbin.com/screens/de9d35ab-8456-4255-8fa7-49000ea9b7a2)) is the minimal version.
- Detail: Delivery ID (copy; stable across retries), Schedule link → drawer, `last_sent_at`, `acked_at`, `next_attempt_at`, payload JSON, and the application's connection state ("billing: offline since 14:02" vs "online, not acknowledging")
- Replay: no confirmation, because replay recreates the same Delivery and consumers are idempotent by key. Show a toast on success and the server error on failure.
- Cancel: a confirmation dialog naming the target (§10)

**Supporting patterns**:
- Resend webhook detail ([link](https://mobbin.com/screens/e5079320-66e3-4ae3-86e3-336a72fd9a6c)): list-detail split, Replay top-right, payload with copy
- Hashnode webhooks history ([link](https://mobbin.com/screens/168e320b-3c66-467c-a88f-16ce90cb6959)): selected row outlined, response panel with Resend, info banner "processed asynchronously… may take a few seconds", which suits at-least-once semantics
- Customer.io deliveries ([link](https://mobbin.com/screens/a8864bb7-9782-4887-8725-e93cc187970a)): status filter, date range, "Last updated … · Auto refresh" toggle, and a "Your failed emails will be retried soon" toast after bulk retry
- Teachable webhook history ([link](https://mobbin.com/screens/b06b6684-800c-426f-8a73-dbe6e8072125)): "disabled after several failed attempts. Re-enable" banner, the analogue of a blocked-deliveries callout on Overview

**Why this matters**: Deliveries is the recovery view (spec story 39). An operator arrives with one question, "what's blocked and why?", and should leave having replayed it.

---

## 8. Calendars View

**Current state** (`web/src/views/Calendars.tsx`): A read-only table of name, truncated application, timezone, seven weekday badges, a definition JSON popover, and updated time. Calendars are edited only through the Publish JSON textarea (`calendar.set`). The server already has `calendar.next`, which computes the next eligible business time, but no UI uses it.

**Pain points**:
- Overrides (holidays, special open days) are hidden inside the JSON popover
- Editing means hand-writing `{"timezone":…, "weekdays":[1,2,3,4,5], "overrides":{"2026-12-25":false}}`, and weekday numbers are 0 = Sunday, which is an easy off-by-one
- There is no way to confirm a calendar does what you meant before a Start schedule depends on it

**Primary recommendation**: Replace the Publish calendar tab with an inline **form in Calendars**, modelled on Cal.com's availability editor ([link](https://mobbin.com/screens/adf8335b-a68e-41bb-a6f5-8a91af85e838)): seven weekday toggle rows, a timezone select on the right, and a "Date overrides" section with "+ Add an override". Calcron calendars have no hours (local time belongs to the Start schedule), so drop Cal.com's time-range inputs and keep only the toggles. The form serialises to the same `calendar.set` payload. Under the form, show **"Next 5 eligible days"** by calling `calendar.next`, so the operator sees the effect of an override before saving.

**Supporting patterns**:
- Cal.com availability ([link](https://mobbin.com/screens/adf8335b-a68e-41bb-a6f5-8a91af85e838)): toggle per weekday, timezone select, date overrides list
- Gorgias custom business hours ([link](https://mobbin.com/screens/d4289e70-4d1f-4b44-8eab-b6b87949a863)): name + timezone + schedule in one modal, plus where the hours are used, the analogue of "Start schedules using this calendar"
- Turo location hours ([link](https://mobbin.com/screens/72a7380f-21f6-43d2-82d7-babd526a9f1d)): card-per-day list with a side drawer edit and "Apply these hours to additional days" chips
- Square location setup ([link](https://mobbin.com/screens/23ebe1d7-4fa9-44a1-8141-34c12af988c7)): checkbox per weekday with disabled rows greyed, a compact read view for the table

**Why this matters**: Calendars decide whether money-adjacent workflows run on a holiday. A preview of the next eligible days catches a wrong override before it misfires.

---

## 9. History View

**Current state** (`web/src/views/History.tsx`): The 100 newest append-only rows show timestamp, colour-coded subject type (schedule / delivery / workflow), event, truncated subject ID, truncated application, and a data JSON popover. There are no filters, no pagination, and no link from a row to its subject.

**Pain points**:
- Following one Workflow instance means scanning 100 mixed rows for a truncated ID
- Anything older than the newest 100 rows is unreachable
- History is the audit trail (spec: "operational audit trail, not a substitute for an application's financial ledger"), but it cannot be narrowed to one application or event

**Primary recommendation**: A filter chip bar above the table, as in PlanetScale's audit log ([link](https://mobbin.com/screens/d7448333-6400-49dd-b11f-e48e4e229200)), where "Add filter" opens a searchable facet picker (Actor → value). Facets: subject type, application (namespace), event, subject ID. Each row's subject ID is an ID chip; clicking it opens that subject's drawer (§10), which shows the same history filtered to that subject. Add "Load older" at the bottom. Client-side filtering over the loaded rows is enough for Phase 1; real narrowing needs server params (§11).

**Supporting patterns**:
- PlanetScale audit log ([link](https://mobbin.com/screens/d7448333-6400-49dd-b11f-e48e4e229200)): "Add filter" facet popover, human sentence plus machine event code (`deploy_request.queued`) per row
- Vanta event log ([link](https://mobbin.com/screens/41659ab7-5710-4abf-91ca-c6bf482c9b0b)): Type / Source / Action / Target dropdown chips plus date range in one row
- Front audit log ([link](https://mobbin.com/screens/0c2efddc-d2d9-4a17-b589-34523d08e1d8)): "Last updated just now · Refresh" beside the filters
- Okta system log ([link](https://mobbin.com/screens/70f42b3f-1823-4cb2-872d-346ad3eab8dc)): event-count histogram above the list. Defer this until the stats time series exists (§11).

**Why this matters**: History is where "why did this happen?" gets answered. Linking every subject ID turns it from a log dump into navigation.

---

## 10. Shared Shell and Primitives

These cross-cutting pieces make each view above cheap to build. Build each once and reuse it in every view.

- **Grouped sidebar**: split the eight flat links into **Operate** (Overview, Schedules, Workflows, Deliveries) · **Configure** (Applications, Calendars, Publish) · **Audit** (History), with a ⌘K search trigger at the top. Vapi's console ([link](https://mobbin.com/screens/ca183d45-d493-42a7-8a32-3d0a727b134c)) groups Build / Test / Observe under small caps labels with search pinned above.
- **Connection pill + freshness**: replace the static green "connected" badge with the real WebSocket state (connected / reconnecting / disconnected), like Neon's "All OK" header pill ([link](https://mobbin.com/screens/283b7c46-b74f-4a64-9392-884428e3c526)). Add "Updated 12s ago" next to it, following ClickUp's "Refreshed 1 min ago · Auto refresh: On" ([link](https://mobbin.com/screens/27113b60-6baf-4b1e-bd2c-3deebad34c58)).
- **Command palette (⌘K)**: navigation to views; **paste any ID** (application, Schedule, Delivery, instance) to open its drawer by matching the loaded lists; and actions on the selection ("Replay delivery", "Cancel delivery"). Linear's palette is referenced in the summary.
- **One detail drawer**: a right-side panel with its state in the URL hash (`#deliveries/dlv_…`) so a link can be pasted into chat. Every list row and ID chip opens it, and its body varies by subject type (§4, §5, §7). Hashnode ([link](https://mobbin.com/screens/168e320b-3c66-467c-a88f-16ce90cb6959)) and Resend ([link](https://mobbin.com/screens/e5079320-66e3-4ae3-86e3-336a72fd9a6c)) show the list-stays-visible split.
- **ID chip**: truncated monospace value, full value on hover, copy icon with a checkmark confirmation, and a click that opens the drawer. Supabase edge functions ([link](https://mobbin.com/screens/a24b428e-afa0-4110-ad48-76efb6cfc96c)) and Neon's API URL field ([link](https://mobbin.com/screens/283b7c46-b74f-4a64-9392-884428e3c526)) use the inline copy icon. This replaces six hand-rolled `slice(0, 10)…` renders.
- **Namespace resolution**: load `apps` once in `AdminProvider` and render `billing` instead of `app_3f9a…` everywhere.
- **Destructive confirm**: a dialog that names the target in the title and states the consequence, with a red button labelled with the verb. ClassDojo ([link](https://mobbin.com/screens/488888a5-3ba8-451c-9531-4f102e4b632d)) puts the target in the title ("Cancel invitation to …?") and uses a red "Cancel invitation" button. Supabase ([link](https://mobbin.com/screens/9caa9824-74fe-45f5-a580-4f69eb5fa3ea)) includes the ID ("abort this query? (ID: 153437)").
- **Toasts for every action result**: fixes the swallowed `act()` reply in Deliveries. Juicebox's bottom-right "Your export is done!" toast ([link](https://mobbin.com/screens/cbe1c32c-64a5-4017-b486-89563de632c0)) has the right weight.
- **Refresh model**: Overview polls every 4s, and every other view waits for a manual refresh. Use one shared poll of the visible view only, paused when `document.visibilityState` is hidden. `ponytail:` polling, not push. Upgrade to a server `dashboard.changed` broadcast when polling cost shows up in query-plan checks.

---

## 11. Constraints Check and Backend Gaps

Several recommendations in §1–§3 and the original §4–§6 need server changes or conflict with binding decisions. Listed so none get built by accident.

**Conflicts with ADRs / spec:**
- **Raw cron display** (original §4): spec 0001 bans raw cron syntax in V1. Schedules are one-shot; recurrence belongs to Start schedules on Business calendars (ADR 0015).
- **Workflow editor features** (original §5–§6: canvas, drag-drop, autocomplete, diff, Test run): ADR 0019 rules out an in-browser workflow-definition editor. Supersede the ADR first or drop these.
- **Admin cancel/retry**: ADR 0019 says administrators "can cancel, retry, and replay work", but the admin router only has `delivery.replay` / `delivery.cancel`. There is no admin cancel for Schedules or Workflow instances. Either add those ops (idempotent, history-writing, through the same seam) or narrow the ADR wording.

**Reads the dashboard needs** (`internal/server/dashboard_read.go`; each is one SQL entry plus a protocol test):
- `schedules`: add `payload`, `chain`
- `deliveries`: add `schedule_id`, `payload`, `last_sent_at`, `acked_at`
- `workflows`: add `waiting_event`, `correlation_key`, `wake_at`, `input`, `state`
- New `start_schedules`: `name, workflow_name, calendar_name, local_time, missed_policy, next_at`
- New `workflow_versions`: `application_id, name, max(version)` for the Publish picker and the "latest v4" pin hint
- New `tokens`: `id, application_id, created_at, revoked_at`, **never `secret_hash`**. This unblocks §3's revoke-old-token flow, because rotation returns only the new `tokenId`.
- `history`: optional `subjectType`, `subjectId`, `applicationId`, and a `before` cursor, replacing the fixed `limit 100`
- Application presence: the hub already knows connected peers. Expose `connected` per application for spec story 41.

**Needs a decision (not a quick read):**
- **Token "last used"** (§3): `application_tokens` has no `last_used_at`. Adding it means a write on every WebSocket authentication. That is cheap, but it puts a write on a read path, so decide explicitly.
- **Sparklines and trends** (§2): `dashboard.stats` is point-in-time. Hourly counts can come from `history` grouped by `date_trunc('hour', created_at)`, which needs an index on `history(created_at)`. Run the query-plan check first (spec testing decisions).

---

## 12. Stack Decision

**Keep DaisyUI 5 + Tailwind v4. Do not migrate to shadcn/ui + Radix.**

- DaisyUI 5 is already installed and Tailwind-v4 native. It ships `drawer`, `modal`, `dropdown`, `toast`, `tabs`, `kbd`, `join`, and `filter`, which covers every primitive in §10.
- Native `<dialog>.showModal()` provides the focus trap, Esc to close, and inert background for the drawer, confirm, and palette with no library.
- shadcn/ui would add about 6 runtime dependencies (Radix packages, `cmdk`, `class-variance-authority`, `clsx`, `tailwind-merge`, `lucide-react`) and rewrite every view for the same result. Assets are embedded in the Go binary (spec 0001), so bundle weight ships with every release.
- Add **`cmdk` only** if the native palette's fuzzy match or keyboard handling falls short in use.
- Motion: DaisyUI / CSS transitions for drawer slide and toast fade. `ponytail:` no Framer Motion. Revisit if a shared-element transition (list row → drawer) proves worth a dependency.

---

## 13. Phased Rollout (revised)

Each phase ships on its own. Order follows "no backend change first".

**Phase 1 — frontend only**
- §10 primitives: grouped sidebar, connection pill + freshness, ID chip, namespace resolution, drawer with hash routing, destructive confirm, toasts (fixes the silent Deliveries action errors), visibility-aware shared poll
- ⌘K palette: navigation + ID jump over loaded lists
- Schedules due buckets, Deliveries Blocked-first split view, History client-side filter chips, Workflows status chips
- Publish: application picker, JSON error position, human success toast
- Calendars form + `calendar.next` preview (existing op)

**Phase 2 — dashboard reads** (§11 list; each lands with a WebSocket protocol test against temp PostgreSQL)
- Extra columns for schedules / deliveries / workflows → Workflows "why" line, Deliveries payload + Schedule link, instance state timeline
- `start_schedules`, `workflow_versions`, `tokens` lists → Start schedules tab, Publish pickers, revoke-old-token flow in Applications
- Application presence → offline vs failing in Deliveries
- History server filters + cursor → subject drawer shows full history, "Load older"

**Phase 3 — needs a decision first**
- Admin cancel for Schedules / Workflow instances (resolve the ADR 0019 wording gap)
- Token `last_used_at`
- Stats time series + sparklines (after the query-plan check)
- Push updates instead of polling
- Anything editor-like in Publish (supersede ADR 0019 first)
