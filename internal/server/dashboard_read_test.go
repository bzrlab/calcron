package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestDashboardWorkflowRowExplainsWhatItWaitsFor(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "why")
	publish(t, ctx, admin, appID, "wait", map[string]any{"initial": "wait", "states": map[string]any{
		"wait": map[string]any{"type": "wait_signal", "event": "payment.confirmed", "correlationKey": "order-7", "next": "end"},
		"end":  map[string]any{"type": "end"},
	}})
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "workflow.start", "name": "wait", "idempotencyKey": "why-start", "data": map[string]any{"order": 7}})
	started := read(t, ctx, app)
	if started["ok"] != true {
		t.Fatalf("start: %#v", started)
	}
	row := rowWhere(t, dashboardRows(t, ctx, admin, "workflows"), "application_id", appID)
	if row["waiting_event"] != "payment.confirmed" || row["correlation_key"] != "order-7" {
		t.Fatalf("workflow row hides what it waits for: %#v", row)
	}
	if input, _ := row["input"].(map[string]any); input["order"] != float64(7) {
		t.Fatalf("workflow row input: %#v", row)
	}
	if _, found := row["wake_at"]; !found {
		t.Fatalf("workflow row missing wake_at: %#v", row)
	}
}

func TestDashboardDeliveryRowShowsPayloadScheduleAndAcknowledgement(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "delivery-detail")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "schedule.set", "idempotencyKey": "detail-set", "key": "order:9", "event": "order.close", "after": "1ms", "data": map[string]any{"orderId": 9}})
	set := read(t, ctx, app)
	if set["ok"] != true {
		t.Fatalf("set: %#v", set)
	}
	got := delivery(t, ctx, app)
	write(t, ctx, app, map[string]any{"op": "delivery.ack", "deliveryId": got["deliveryId"], "idempotencyKey": "detail-ack"})
	if ack := read(t, ctx, app); ack["ok"] != true {
		t.Fatalf("ack: %#v", ack)
	}
	row := waitForDeliveryStatus(t, ctx, admin, got["deliveryId"].(string), "acked")
	if row["schedule_id"] != set["data"].(map[string]any)["scheduleId"] {
		t.Fatalf("delivery row not linked to schedule: %#v", row)
	}
	if payload, _ := row["payload"].(map[string]any); payload["orderId"] != float64(9) {
		t.Fatalf("delivery row payload: %#v", row)
	}
	if row["last_sent_at"] == nil || row["acked_at"] == nil {
		t.Fatalf("delivery row missing send/ack times: %#v", row)
	}
	schedule := rowWhere(t, dashboardRows(t, ctx, admin, "schedules"), "application_id", appID)
	if payload, _ := schedule["payload"].(map[string]any); payload["orderId"] != float64(9) {
		t.Fatalf("schedule row payload: %#v", schedule)
	}
	if _, found := schedule["chain"]; !found {
		t.Fatalf("schedule row missing chain: %#v", schedule)
	}
}

func TestDashboardWorkflowDeliveryLinksItsInstance(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "emit-link")
	publish(t, ctx, admin, appID, "emit", map[string]any{"initial": "emit", "states": map[string]any{
		"emit": map[string]any{"type": "emit", "target": appID, "event": "order.ship", "next": "end"},
		"end":  map[string]any{"type": "end"},
	}})
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "workflow.start", "name": "emit", "idempotencyKey": "emit-start"})
	started := read(t, ctx, app)
	if started["ok"] != true {
		t.Fatalf("start: %#v", started)
	}
	row := rowWhere(t, dashboardRows(t, ctx, admin, "deliveries"), "application_id", appID)
	if row["schedule_id"] != nil || row["workflow_instance_id"] != started["data"].(map[string]any)["instanceId"] {
		t.Fatalf("workflow delivery not linked to its instance: %#v", row)
	}
}

func TestDashboardTokensListShowsRevocationWithoutSecrets(t *testing.T) {
	ctx, _, admin := testServer(t, 0)
	appID, _ := newApp(t, ctx, admin, "token-list")
	write(t, ctx, admin, map[string]any{"op": "app.token.rotate", "applicationId": appID})
	rotated := read(t, ctx, admin)
	if rotated["ok"] != true {
		t.Fatalf("rotate: %#v", rotated)
	}
	replacementID := rotated["data"].(map[string]any)["tokenId"]
	write(t, ctx, admin, map[string]any{"op": "app.token.revoke", "tokenId": replacementID})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("revoke: %#v", got)
	}
	var mine []map[string]any
	for _, row := range dashboardRows(t, ctx, admin, "tokens") {
		if row["application_id"] == appID {
			mine = append(mine, row)
		}
	}
	if len(mine) != 2 {
		t.Fatalf("tokens for app = %d, want 2: %#v", len(mine), mine)
	}
	for _, row := range mine {
		if _, leaked := row["secret_hash"]; leaked {
			t.Fatal("tokens list exposes secret_hash")
		}
		revoked := row["revoked_at"] != nil
		if revoked != (row["id"] == replacementID) {
			t.Fatalf("token revocation state wrong: %#v", row)
		}
	}
}

func TestDashboardListsStartSchedulesAndLatestWorkflowVersions(t *testing.T) {
	ctx, _, admin := testServer(t, 0)
	appID, _ := newApp(t, ctx, admin, "config-lists")
	definition := map[string]any{"initial": "end", "states": map[string]any{"end": map[string]any{"type": "end"}}}
	publish(t, ctx, admin, appID, "settle", definition)
	publish(t, ctx, admin, appID, "settle", definition)
	write(t, ctx, admin, map[string]any{"op": "calendar.set", "applicationId": appID, "name": "weekdays", "data": map[string]any{"timezone": "UTC", "weekdays": []int{1, 2, 3, 4, 5}}})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("calendar.set: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "start-schedule.set", "applicationId": appID, "name": "morning", "workflow": "settle", "calendar": "weekdays", "localTime": "09:30", "missedPolicy": "run_once_late"})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("start-schedule.set: %#v", got)
	}
	version := rowWhere(t, dashboardRows(t, ctx, admin, "workflow_versions"), "application_id", appID)
	if version["name"] != "settle" || version["version"] != float64(2) {
		t.Fatalf("latest workflow version: %#v", version)
	}
	start := rowWhere(t, dashboardRows(t, ctx, admin, "start_schedules"), "application_id", appID)
	if start["name"] != "morning" || start["workflow_name"] != "settle" || start["calendar_name"] != "weekdays" ||
		start["local_time"] != "09:30" || start["missed_policy"] != "run_once_late" || start["next_at"] == nil {
		t.Fatalf("start schedule row: %#v", start)
	}
}

func TestDashboardAppsShowConnectionPresence(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "presence")
	if row := rowWhere(t, dashboardRows(t, ctx, admin, "apps"), "id", appID); row["connected"] != false {
		t.Fatalf("offline application shown connected: %#v", row)
	}
	app := dial(t, ctx, h.URL, token)
	if row := rowWhere(t, dashboardRows(t, ctx, admin, "apps"), "id", appID); row["connected"] != true {
		t.Fatalf("connected application shown offline: %#v", row)
	}
	app.CloseNow()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rowWhere(t, dashboardRows(t, ctx, admin, "apps"), "id", appID)["connected"] == false {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("disconnected application still shown connected")
}

func TestDashboardHistoryFiltersBySubjectAndPagesOlder(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "history-filter")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	var scheduleID string
	for i, key := range []string{"a", "b", "c"} {
		write(t, ctx, app, map[string]any{"op": "schedule.set", "idempotencyKey": "hist-" + key, "key": "order:" + key, "event": "order.close", "after": "1h"})
		got := read(t, ctx, app)
		if got["ok"] != true {
			t.Fatalf("set %s: %#v", key, got)
		}
		if i == 1 {
			scheduleID = got["data"].(map[string]any)["scheduleId"].(string)
		}
	}
	subject := historyRows(t, ctx, admin, map[string]any{"subjectType": "schedule", "subjectId": scheduleID})
	if len(subject) != 1 || subject[0]["subject_id"] != scheduleID {
		t.Fatalf("subject filter returned %#v", subject)
	}
	mine := historyRows(t, ctx, admin, map[string]any{"applicationId": appID})
	if len(mine) != 3 {
		t.Fatalf("application filter rows = %d, want 3", len(mine))
	}
	older := historyRows(t, ctx, admin, map[string]any{"applicationId": appID, "before": mine[0]["id"]})
	if len(older) != 2 || older[0]["id"] != mine[1]["id"] {
		t.Fatalf("before cursor returned %#v, want rows after %v", older, mine[0]["id"])
	}
}

func historyRows(t *testing.T, ctx context.Context, admin *websocket.Conn, filter map[string]any) []map[string]any {
	t.Helper()
	frame := map[string]any{"op": "dashboard.list", "name": "history"}
	for k, v := range filter {
		frame[k] = v
	}
	write(t, ctx, admin, frame)
	got := read(t, ctx, admin)
	if got["ok"] != true {
		t.Fatalf("history %v: %#v", filter, got)
	}
	data, _ := got["data"].([]any)
	rows := make([]map[string]any, len(data))
	for i, row := range data {
		rows[i] = row.(map[string]any)
	}
	return rows
}

func publish(t *testing.T, ctx context.Context, admin *websocket.Conn, appID, name string, definition map[string]any) {
	t.Helper()
	write(t, ctx, admin, map[string]any{"op": "workflow.publish", "applicationId": appID, "name": name, "data": definition})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("workflow.publish %s: %#v", name, got)
	}
}

func rowWhere(t *testing.T, rows []map[string]any, key string, value any) map[string]any {
	t.Helper()
	for _, row := range rows {
		if row[key] == value {
			return row
		}
	}
	t.Fatalf("no row with %s=%v in %d rows", key, value, len(rows))
	return nil
}
