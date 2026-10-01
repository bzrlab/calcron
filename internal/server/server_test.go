package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/bzrlab/calcron"
	"github.com/bzrlab/calcron/internal/server"
	"github.com/coder/websocket"
)

func TestScheduleDeliversOverWebSocket(t *testing.T) {
	db := os.Getenv("CALCRON_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("CALCRON_TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := server.New(ctx, server.Config{DatabaseURL: db, AdminToken: "test-admin", Migration: calcron.InitialMigration})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	admin := dial(t, ctx, h.URL, "test-admin")
	defer admin.CloseNow()
	write(t, ctx, admin, map[string]any{"id": "new-app", "op": "app.create", "name": "test", "namespace": "test-" + time.Now().Format("150405.000000000")})
	created := read(t, ctx, admin)
	if created["ok"] != true {
		t.Fatalf("app.create: %#v", created)
	}
	token := created["data"].(map[string]any)["token"].(string)
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"id": "missing-key", "op": "schedule.cancel", "key": "ticket:1:close"})
	if got := read(t, ctx, app); got["error"] != "idempotencyKey required" {
		t.Fatalf("missing key: %#v", got)
	}
	write(t, ctx, app, map[string]any{"id": "set", "op": "schedule.set", "idempotencyKey": "one", "key": "ticket:1:close", "event": "ticket.close", "after": "10ms", "data": map[string]string{"ticketId": "1"}, "chain": map[string]any{"key": "ticket:1:escalate", "event": "ticket.escalate", "after": "10ms"}})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("set: %#v", got)
	}
	first := delivery(t, ctx, app)
	if first["event"] != "ticket.close" {
		t.Fatalf("delivery: %#v", first)
	}
	write(t, ctx, app, map[string]any{"id": "ack", "op": "delivery.ack", "deliveryId": first["deliveryId"], "idempotencyKey": "ack-one"})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("ack: %#v", got)
	}
	write(t, ctx, app, map[string]any{"id": "ack-retry", "op": "delivery.ack", "deliveryId": first["deliveryId"], "idempotencyKey": "ack-one"})
	if got := read(t, ctx, app); got["ok"] != true || got["data"].(map[string]any)["acked"] != true {
		t.Fatalf("ack retry: %#v", got)
	}
	second := delivery(t, ctx, app)
	if second["event"] != "ticket.escalate" {
		t.Fatalf("chain: %#v", second)
	}
}

func TestWorkflowWaitsForCorrelatedSignal(t *testing.T) {
	db := os.Getenv("CALCRON_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("CALCRON_TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := server.New(ctx, server.Config{DatabaseURL: db, AdminToken: "test-admin", Migration: calcron.InitialMigration})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	admin := dial(t, ctx, h.URL, "test-admin")
	defer admin.CloseNow()
	write(t, ctx, admin, map[string]any{"op": "app.create", "name": "workflow", "namespace": "workflow-" + time.Now().Format("150405.000000000")})
	created := read(t, ctx, admin)
	if created["ok"] != true {
		t.Fatalf("app.create: %#v", created)
	}
	data := created["data"].(map[string]any)
	appID, token := data["applicationId"].(string), data["token"].(string)
	write(t, ctx, admin, map[string]any{"op": "app.token.rotate", "applicationId": appID})
	rotated := read(t, ctx, admin)
	if rotated["ok"] != true {
		t.Fatalf("rotate: %#v", rotated)
	}
	write(t, ctx, admin, map[string]any{"op": "app.token.revoke", "tokenId": rotated["data"].(map[string]any)["tokenId"]})
	if got := read(t, ctx, admin); got["ok"] != true || got["data"].(map[string]any)["revoked"] != true {
		t.Fatalf("revoke: %#v", got)
	}
	definition := map[string]any{"initial": "wait", "states": map[string]any{
		"wait":   map[string]any{"type": "wait_signal", "event": "payment.confirmed", "correlationKey": "order-1", "next": "branch"},
		"branch": map[string]any{"type": "branch", "when": "state.signal.ok == true", "true": "emit", "false": "end"},
		"emit":   map[string]any{"type": "emit", "target": appID, "event": "payment.close", "next": "end"},
		"end":    map[string]any{"type": "end"},
	}}
	write(t, ctx, admin, map[string]any{"op": "workflow.publish", "applicationId": appID, "name": "payment", "data": definition})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("publish: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "calendar.set", "applicationId": appID, "name": "weekday", "data": map[string]any{"timezone": "Asia/Dhaka", "weekdays": []int{0, 1, 2, 3, 4, 5, 6}}})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("calendar: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "start-schedule.set", "applicationId": appID, "name": "daily", "workflow": "payment", "calendar": "weekday", "localTime": "09:00", "missedPolicy": "skip"})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("start schedule: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "dashboard.stats"})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("stats: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "dashboard.list", "name": "history"})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("history: %#v", got)
	}
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "workflow.start", "name": "payment", "idempotencyKey": "start"})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("start: %#v", got)
	}
	write(t, ctx, app, map[string]any{"op": "signal", "event": "payment.confirmed", "correlationKey": "order-1", "data": map[string]bool{"ok": true}, "idempotencyKey": "signal"})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("signal: %#v", got)
	}
	got := delivery(t, ctx, app)
	if got["event"] != "payment.close" {
		t.Fatalf("delivery: %#v", got)
	}
}

func TestChainWaitsItsDelayAfterAcknowledgement(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "chain-delay")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "schedule.set", "idempotencyKey": "warn", "key": "ticket:9:idle", "event": "ticket.idle", "after": "1ms", "chain": map[string]any{"key": "ticket:9:close", "event": "ticket.close", "after": "1h"}})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("set: %#v", got)
	}
	warn := delivery(t, ctx, app)
	write(t, ctx, app, map[string]any{"op": "delivery.ack", "deliveryId": warn["deliveryId"], "idempotencyKey": "warn-ack"})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("ack: %#v", got)
	}
	var runAt time.Time
	for _, row := range listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID}) {
		if row["schedule_key"] == "ticket:9:close" {
			runAt, _ = time.Parse(time.RFC3339Nano, row["run_at"].(string))
		}
	}
	if time.Until(runAt) < 59*time.Minute {
		t.Fatalf("chain run_at = %v, want about one hour from now", runAt)
	}
}

func TestDashboardListFiltersByApplication(t *testing.T) {
	ctx, _, admin := testServer(t, 0)
	mine, _ := newApp(t, ctx, admin, "filter-mine")
	other, _ := newApp(t, ctx, admin, "filter-other")
	for _, app := range []string{mine, other} {
		write(t, ctx, admin, map[string]any{"op": "calendar.set", "applicationId": app, "name": "workdays", "data": map[string]any{"timezone": "UTC", "weekdays": []int{1}}})
		if got := read(t, ctx, admin); got["ok"] != true {
			t.Fatalf("calendar: %#v", got)
		}
	}
	write(t, ctx, admin, map[string]any{"op": "dashboard.list", "name": "calendars", "applicationId": mine})
	got := read(t, ctx, admin)
	rows, _ := got["data"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["application_id"] != mine {
		t.Fatalf("filtered calendars: %#v", got)
	}
}

func TestSignalAdvancesOnlyInstanceWithDerivedCorrelationKey(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "correlation")
	definition := map[string]any{"initial": "wait", "states": map[string]any{
		"wait": map[string]any{"type": "wait_signal", "event": "ticket.resolved", "correlationKeyExpr": "'ticket:' + input.ticketId", "next": "emit"},
		"emit": map[string]any{"type": "emit", "target": appID, "event": "ticket.closed", "dataExpr": "input", "next": "end"},
		"end":  map[string]any{"type": "end"},
	}}
	write(t, ctx, admin, map[string]any{"op": "workflow.publish", "applicationId": appID, "name": "ticket", "data": definition})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("publish: %#v", got)
	}
	versions := listRows(t, ctx, admin, map[string]any{"name": "workflow_versions", "applicationId": appID})
	if len(versions) != 1 || versions[0]["definition"] == nil {
		t.Fatalf("workflow version row lacks definition: %#v", versions)
	}
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	for _, id := range []string{"1", "2"} {
		write(t, ctx, app, map[string]any{"op": "workflow.start", "name": "ticket", "data": map[string]string{"ticketId": id}, "idempotencyKey": "start-" + id})
		if got := read(t, ctx, app); got["ok"] != true {
			t.Fatalf("start %s: %#v", id, got)
		}
	}
	write(t, ctx, app, map[string]any{"op": "signal", "event": "ticket.resolved", "correlationKey": "ticket:2", "idempotencyKey": "resolve-2"})
	if got := read(t, ctx, app); got["ok"] != true || got["data"].(map[string]any)["matched"] != float64(1) {
		t.Fatalf("signal: %#v", got)
	}
	if got := delivery(t, ctx, app); got["event"] != "ticket.closed" || got["data"].(map[string]any)["ticketId"] != "2" {
		t.Fatalf("delivery: %#v", got)
	}
}

func TestWorkflowPublishRejectsInvalidCorrelationKeyExpr(t *testing.T) {
	ctx, _, admin := testServer(t, 0)
	appID, _ := newApp(t, ctx, admin, "correlation-invalid")
	for _, wait := range []map[string]any{
		{"type": "wait_signal", "event": "e", "correlationKeyExpr": "input.id +", "next": "end"},
		{"type": "wait_signal", "event": "e", "correlationKey": "k", "correlationKeyExpr": "input.id", "next": "end"},
	} {
		write(t, ctx, admin, map[string]any{"op": "workflow.publish", "applicationId": appID, "name": "bad", "data": map[string]any{"initial": "wait", "states": map[string]any{"wait": wait, "end": map[string]any{"type": "end"}}}})
		if got := read(t, ctx, admin); got["ok"] == true {
			t.Fatalf("accepted %#v", wait)
		}
	}
}

func TestExtendCancelsDeliveryQueuedForOldDeadline(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "extend-pending")
	app := dial(t, ctx, h.URL, token)
	write(t, ctx, app, map[string]any{"op": "schedule.set", "idempotencyKey": "set", "key": "giveaway:1", "event": "giveaway.end", "after": "1ms"})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("set: %#v", got)
	}
	app.CloseNow()
	time.Sleep(600 * time.Millisecond)
	queued := listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID, "status": "pending"})
	if len(queued) != 1 {
		t.Fatalf("queued deliveries: %#v", queued)
	}
	app = dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "schedule.extend", "idempotencyKey": "extend", "key": "giveaway:1", "by": "1h"})
	got := read(t, ctx, app)
	for got["op"] == "delivery" {
		got = read(t, ctx, app)
	}
	if got["ok"] != true {
		t.Fatalf("extend: %#v", got)
	}
	if got := listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID, "status": "cancelled"}); len(got) != 1 || got[0]["id"] != queued[0]["id"] || got[0]["status"] != "cancelled" {
		t.Fatalf("stale delivery not cancelled: %#v", got)
	}
}

func TestWorkflowFailsVisiblyWhenConditionErrors(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "cel-fault")
	publish(t, ctx, admin, appID, "fault", map[string]any{"initial": "wait", "states": map[string]any{
		"wait": map[string]any{"type": "wait_signal", "event": "e", "correlationKeyExpr": "'k:' + input.missing", "next": "end"},
		"end":  map[string]any{"type": "end"},
	}})
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "workflow.start", "name": "fault", "idempotencyKey": "start"})
	started := read(t, ctx, app)
	if started["ok"] != true {
		t.Fatalf("start: %#v", started)
	}
	id := started["data"].(map[string]any)["instanceId"]
	if rows := listRows(t, ctx, admin, map[string]any{"name": "workflows", "applicationId": appID}); len(rows) != 1 || rows[0]["status"] != "failed" {
		t.Fatalf("instance not failed: %#v", rows)
	}
	history := listRows(t, ctx, admin, map[string]any{"name": "history", "subjectType": "workflow", "subjectId": id})
	if len(history) == 0 || history[0]["event"] != "failed" || history[0]["data"].(map[string]any)["error"] == "" {
		t.Fatalf("failure not recorded: %#v", history)
	}
}

func TestScheduleIdempotencyReturnsOriginalResult(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, token := newApp(t, ctx, admin, "idempotency")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	command := map[string]any{"op": "schedule.set", "idempotencyKey": "set-once", "key": "order:1", "event": "order.close", "after": "1h"}
	write(t, ctx, app, command)
	first := read(t, ctx, app)
	write(t, ctx, app, command)
	second := read(t, ctx, app)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("idempotent response changed: first=%#v second=%#v", first, second)
	}
	scheduleID := first["data"].(map[string]any)["scheduleId"]
	count := 0
	for _, row := range dashboardRows(t, ctx, admin, "history") {
		if row["subject_id"] == scheduleID && row["event"] == "set" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("schedule set history entries = %d, want 1", count)
	}
}

func TestOfflineDeliveryDoesNotRetryAndReconnectsWithSameID(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "offline")
	app := dial(t, ctx, h.URL, token)
	write(t, ctx, app, map[string]any{"op": "schedule.set", "idempotencyKey": "offline-set", "key": "order:offline", "event": "order.close", "after": "1ms"})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("set: %#v", got)
	}
	app.CloseNow()
	time.Sleep(600 * time.Millisecond)
	var row map[string]any
	for _, candidate := range dashboardRows(t, ctx, admin, "deliveries") {
		if candidate["application_id"] == appID {
			row = candidate
			break
		}
	}
	if row == nil {
		t.Fatal("offline delivery not found")
	}
	if row["status"] != "pending" || row["attempts"] != float64(0) {
		t.Fatalf("offline delivery: %#v", row)
	}
	reconnected := dial(t, ctx, h.URL, token)
	defer reconnected.CloseNow()
	if got := delivery(t, ctx, reconnected); got["deliveryId"] != row["id"] {
		t.Fatalf("delivery ID changed after reconnect: got=%#v want=%#v", got, row["id"])
	}
}

func TestBlockedDeliveryCanReplayAndCancel(t *testing.T) {
	retryBase := 100 * time.Millisecond
	ctx, h, admin := testServer(t, retryBase)
	_, token := newApp(t, ctx, admin, "recovery")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "schedule.set", "idempotencyKey": "recovery-set", "key": "order:recovery", "event": "order.close", "after": "1ms"})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("set: %#v", got)
	}
	var first map[string]any
	for attempt := range 5 {
		got := delivery(t, ctx, app)
		if first == nil {
			first = got
		} else if got["deliveryId"] != first["deliveryId"] {
			t.Fatalf("retry changed delivery ID: first=%#v got=%#v", first, got)
		}
		row := waitForDeliveryAttempts(t, ctx, admin, first["deliveryId"].(string), float64(attempt+1))
		next, err := time.Parse(time.RFC3339Nano, row["next_attempt_at"].(string))
		if err != nil {
			t.Fatalf("next attempt timestamp: %v", err)
		}
		expected := retryBase * time.Duration(1<<attempt)
		if remaining := time.Until(next); remaining < expected/2 {
			t.Fatalf("attempt %d retry delay = %v, want at least %v", attempt+1, remaining, expected/2)
		}
	}
	deliveryID := first["deliveryId"].(string)
	row := waitForDeliveryStatus(t, ctx, admin, deliveryID, "blocked")
	write(t, ctx, admin, map[string]any{"op": "delivery.replay", "deliveryId": first["deliveryId"]})
	if got := read(t, ctx, admin); got["ok"] != true || got["data"].(map[string]any)["replayed"] != true {
		t.Fatalf("replay: %#v", got)
	}
	if got := delivery(t, ctx, app); got["deliveryId"] != first["deliveryId"] {
		t.Fatalf("replay changed delivery ID: first=%#v got=%#v", first, got)
	}
	write(t, ctx, admin, map[string]any{"op": "delivery.cancel", "deliveryId": first["deliveryId"]})
	if got := read(t, ctx, admin); got["ok"] != true || got["data"].(map[string]any)["cancelled"] != true {
		t.Fatalf("cancel: %#v", got)
	}
	if got := waitForDeliveryStatus(t, ctx, admin, row["id"].(string), "cancelled"); got["status"] != "cancelled" {
		t.Fatalf("cancelled delivery: %#v", got)
	}
}

func TestTokenRevocationPreservesOverlappingTokenAndBlocksAdminOps(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, original := newApp(t, ctx, admin, "tokens")
	write(t, ctx, admin, map[string]any{"op": "app.token.rotate", "applicationId": appID})
	rotated := read(t, ctx, admin)
	if rotated["ok"] != true {
		t.Fatalf("rotate: %#v", rotated)
	}
	data := rotated["data"].(map[string]any)
	replacement := data["token"].(string)
	if app := dial(t, ctx, h.URL, original); app == nil {
		t.Fatal("original token rejected during rotation overlap")
	} else {
		defer app.CloseNow()
		write(t, ctx, app, map[string]any{"op": "app.create", "idempotencyKey": "forbidden"})
		if got := read(t, ctx, app); got["error"] != "unknown op" {
			t.Fatalf("application ran admin command: %#v", got)
		}
	}
	app := dial(t, ctx, h.URL, replacement)
	app.CloseNow()
	write(t, ctx, admin, map[string]any{"op": "app.token.revoke", "tokenId": data["tokenId"]})
	if got := read(t, ctx, admin); got["ok"] != true || got["data"].(map[string]any)["revoked"] != true {
		t.Fatalf("revoke: %#v", got)
	}
	dialUnauthorized(t, ctx, h.URL, replacement)
	app = dial(t, ctx, h.URL, original)
	defer app.CloseNow()
}

func TestApplicationCannotAcknowledgeAnotherApplicationsDelivery(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, firstToken := newApp(t, ctx, admin, "first")
	_, secondToken := newApp(t, ctx, admin, "second")
	first := dial(t, ctx, h.URL, firstToken)
	defer first.CloseNow()
	second := dial(t, ctx, h.URL, secondToken)
	defer second.CloseNow()
	write(t, ctx, second, map[string]any{"op": "schedule.set", "idempotencyKey": "second-set", "key": "order:second", "event": "order.close", "after": "1ms"})
	if got := read(t, ctx, second); got["ok"] != true {
		t.Fatalf("set: %#v", got)
	}
	delivered := delivery(t, ctx, second)
	write(t, ctx, first, map[string]any{"op": "delivery.ack", "idempotencyKey": "first-ack", "deliveryId": delivered["deliveryId"]})
	if got := read(t, ctx, first); got["ok"] != true || got["data"].(map[string]any)["acked"] != false {
		t.Fatalf("first application acknowledged second delivery: %#v", got)
	}
	write(t, ctx, second, map[string]any{"op": "delivery.ack", "idempotencyKey": "second-ack", "deliveryId": delivered["deliveryId"]})
	if got := read(t, ctx, second); got["ok"] != true || got["data"].(map[string]any)["acked"] != true {
		t.Fatalf("second application acknowledgement: %#v", got)
	}
}

func TestCalendarNextHonorsOverridesAndDST(t *testing.T) {
	ctx, _, admin := testServer(t, 0)
	appID, _ := newApp(t, ctx, admin, "calendar")
	write(t, ctx, admin, map[string]any{"op": "calendar.set", "applicationId": appID, "name": "ny", "data": map[string]any{
		"timezone": "America/New_York", "weekdays": []int{1}, "overrides": map[string]bool{"2025-01-04": true, "2025-01-06": false},
	}})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("calendar.set: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "calendar.next", "applicationId": appID, "calendar": "ny", "localTime": "09:00", "at": "2025-01-03T12:00:00Z"})
	if got := read(t, ctx, admin); got["ok"] != true || got["data"].(map[string]any)["nextAt"] != "2025-01-04T14:00:00Z" {
		t.Fatalf("calendar override: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "calendar.next", "applicationId": appID, "calendar": "ny", "localTime": "09:00", "at": "2025-01-04T15:00:00Z"})
	if got := read(t, ctx, admin); got["ok"] != true || got["data"].(map[string]any)["nextAt"] != "2025-01-13T14:00:00Z" {
		t.Fatalf("calendar excluded weekday: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "calendar.set", "applicationId": appID, "name": "dst", "data": map[string]any{"timezone": "America/New_York", "weekdays": []int{0}}})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("dst calendar.set: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "calendar.next", "applicationId": appID, "calendar": "dst", "localTime": "02:30", "at": "2025-03-08T00:00:00Z"})
	if got := read(t, ctx, admin); got["error"] != "localTime does not exist on calendar date" {
		t.Fatalf("DST gap accepted: %#v", got)
	}
}

func TestCalendarOccurrences(t *testing.T) {
	ctx, _, admin := testServer(t, 0)
	appID, _ := newApp(t, ctx, admin, "occurrences")
	write(t, ctx, admin, map[string]any{"op": "calendar.set", "applicationId": appID, "name": "ny", "data": map[string]any{
		"timezone": "America/New_York", "weekdays": []int{1}, "overrides": map[string]bool{"2025-01-04": true, "2025-01-06": false},
	}})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("calendar.set: %#v", got)
	}
	occurrences := func(got map[string]any) []any {
		t.Helper()
		if got["ok"] != true {
			t.Fatalf("calendar.occurrences: %#v", got)
		}
		return got["data"].(map[string]any)["occurrences"].([]any)
	}

	write(t, ctx, admin, map[string]any{"op": "calendar.occurrences", "applicationId": appID, "calendar": "ny", "localTime": "09:00", "at": "2025-01-04T14:00:00Z", "until": "2025-01-20T14:00:00Z"})
	if got := fmt.Sprint(occurrences(read(t, ctx, admin))); got != "[2025-01-04T14:00:00Z 2025-01-13T14:00:00Z]" {
		t.Fatalf("named range must include at, skip the closed override, and exclude until: %s", got)
	}

	write(t, ctx, admin, map[string]any{"op": "calendar.occurrences", "localTime": "09:00", "at": "2025-01-01T00:00:00Z", "until": "2025-03-01T00:00:00Z",
		"data": map[string]any{"timezone": "UTC", "weekdays": []int{0, 1, 2, 3, 4, 5, 6}}})
	if got := len(occurrences(read(t, ctx, admin))); got != 59 {
		t.Fatalf("inline daily calendar over 59 days returned %d runs", got)
	}

	write(t, ctx, admin, map[string]any{"op": "calendar.occurrences", "localTime": "02:30", "at": "2025-03-08T00:00:00Z", "until": "2025-03-11T00:00:00Z",
		"data": map[string]any{"timezone": "America/New_York", "weekdays": []int{0, 1, 2, 3, 4, 5, 6}}})
	if got := fmt.Sprint(occurrences(read(t, ctx, admin))); got != "[2025-03-08T07:30:00Z 2025-03-10T06:30:00Z]" {
		t.Fatalf("spring-forward date must be skipped, not fail the range: %s", got)
	}

	for name, f := range map[string]map[string]any{
		"range over a year": {"calendar": "ny", "applicationId": appID, "at": "2025-01-01T00:00:00Z", "until": "2026-01-03T00:00:00Z"},
		"until before at":   {"calendar": "ny", "applicationId": appID, "at": "2025-01-02T00:00:00Z", "until": "2025-01-01T00:00:00Z"},
		"no calendar":       {"at": "2025-01-01T00:00:00Z", "until": "2025-01-02T00:00:00Z"},
		"named without app": {"calendar": "ny", "at": "2025-01-01T00:00:00Z", "until": "2025-01-02T00:00:00Z"},
		"invalid inline":    {"data": map[string]any{"timezone": "Mars/Base", "weekdays": []int{1}}, "at": "2025-01-01T00:00:00Z", "until": "2025-01-02T00:00:00Z"},
	} {
		f["op"], f["localTime"] = "calendar.occurrences", "09:00"
		write(t, ctx, admin, f)
		if got := read(t, ctx, admin); got["ok"] != false {
			t.Fatalf("%s accepted: %#v", name, got)
		}
	}
}

func TestMissedOccurrencePolicies(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "missed")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, admin, map[string]any{"op": "calendar.set", "applicationId": appID, "name": "daily", "data": map[string]any{"timezone": "UTC", "weekdays": []int{0, 1, 2, 3, 4, 5, 6}}})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("calendar.set: %#v", got)
	}
	definition := map[string]any{"initial": "emit", "states": map[string]any{
		"emit": map[string]any{"type": "emit", "target": appID, "event": "missed.run", "next": "end"},
		"end":  map[string]any{"type": "end"},
	}}
	write(t, ctx, admin, map[string]any{"op": "workflow.publish", "applicationId": appID, "name": "missed", "data": definition})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("workflow.publish: %#v", got)
	}
	clock := time.Now().UTC().Format("15:04")
	at := time.Now().UTC().AddDate(0, 0, -2).Truncate(time.Minute).Format(time.RFC3339)
	for _, policy := range []string{"skip", "run_once_late", "catch_up"} {
		write(t, ctx, admin, map[string]any{"op": "start-schedule.set", "applicationId": appID, "name": policy, "workflow": "missed", "calendar": "daily", "localTime": clock, "missedPolicy": policy, "at": at})
		if got := read(t, ctx, admin); got["ok"] != true {
			t.Fatalf("%s start schedule: %#v", policy, got)
		}
	}
	if got := waitForWorkflowCount(t, ctx, admin, appID, "missed", 4); got != 4 {
		t.Fatalf("missed workflows = %d, want skip=0 run_once_late=1 catch_up=3", got)
	}
}

func testServer(t *testing.T, retryBase time.Duration) (context.Context, *httptest.Server, *websocket.Conn) {
	t.Helper()
	db := os.Getenv("CALCRON_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("CALCRON_TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s, err := server.New(ctx, server.Config{DatabaseURL: db, AdminToken: "test-admin", Migration: calcron.InitialMigration, RetryBase: retryBase})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	h := httptest.NewServer(s.Handler())
	t.Cleanup(h.Close)
	admin := dial(t, ctx, h.URL, "test-admin")
	t.Cleanup(func() { admin.CloseNow() })
	return ctx, h, admin
}

func newApp(t *testing.T, ctx context.Context, admin *websocket.Conn, name string) (string, string) {
	t.Helper()
	write(t, ctx, admin, map[string]any{"op": "app.create", "name": name, "namespace": name + "-" + time.Now().Format("150405.000000000")})
	got := read(t, ctx, admin)
	if got["ok"] != true {
		t.Fatalf("app.create: %#v", got)
	}
	data := got["data"].(map[string]any)
	return data["applicationId"].(string), data["token"].(string)
}

func dashboardRows(t *testing.T, ctx context.Context, admin *websocket.Conn, name string) []map[string]any {
	t.Helper()
	return listRows(t, ctx, admin, map[string]any{"name": name})
}

func listRows(t *testing.T, ctx context.Context, admin *websocket.Conn, request map[string]any) []map[string]any {
	t.Helper()
	request["op"] = "dashboard.list"
	write(t, ctx, admin, request)
	name := request["name"]
	got := read(t, ctx, admin)
	if got["ok"] != true {
		t.Fatalf("dashboard.list %s: %#v", name, got)
	}
	data, _ := got["data"].([]any)
	rows := make([]map[string]any, len(data))
	for i, row := range data {
		rows[i] = row.(map[string]any)
	}
	return rows
}

func workflowCount(t *testing.T, ctx context.Context, admin *websocket.Conn, appID, name string) int {
	t.Helper()
	count := 0
	for _, row := range dashboardRows(t, ctx, admin, "workflows") {
		if row["application_id"] == appID && row["workflow_name"] == name {
			count++
		}
	}
	return count
}

func waitForWorkflowCount(t *testing.T, ctx context.Context, admin *websocket.Conn, appID, name string, want int) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := workflowCount(t, ctx, admin, appID, name); got == want {
			return got
		}
		time.Sleep(25 * time.Millisecond)
	}
	return workflowCount(t, ctx, admin, appID, name)
}

func waitForDeliveryStatus(t *testing.T, ctx context.Context, admin *websocket.Conn, id, status string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, row := range dashboardRows(t, ctx, admin, "deliveries") {
			if row["id"] == id && row["status"] == status {
				return row
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("delivery %s did not become %s", id, status)
	return nil
}

func waitForDeliveryAttempts(t *testing.T, ctx context.Context, admin *websocket.Conn, id string, attempts float64) map[string]any {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, row := range dashboardRows(t, ctx, admin, "deliveries") {
			if row["id"] == id && row["attempts"] == attempts {
				return row
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("delivery %s did not reach %g attempts", id, attempts)
	return nil
}

func dialUnauthorized(t *testing.T, ctx context.Context, rawURL, token string) {
	t.Helper()
	c, _, err := websocket.Dial(ctx, "ws"+rawURL[4:]+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	write(t, ctx, c, map[string]string{"op": "auth", "token": token})
	if got := read(t, ctx, c); got["error"] != "unauthorized" {
		t.Fatal("revoked token authenticated")
	}
}

func delivery(t *testing.T, ctx context.Context, c *websocket.Conn) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got := read(t, ctx, c)
		if got["op"] == "delivery" {
			return got
		}
	}
	t.Fatal("no delivery")
	return nil
}

func dial(t *testing.T, ctx context.Context, rawURL, token string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.Dial(ctx, "ws"+rawURL[4:]+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	write(t, ctx, c, map[string]string{"op": "auth", "token": token})
	if got := read(t, ctx, c); got["ok"] != true {
		t.Fatal("authentication failed")
	}
	return c
}
func write(t *testing.T, ctx context.Context, c *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, ctx context.Context, c *websocket.Conn) map[string]any {
	t.Helper()
	rctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	_, b, err := c.Read(rctx)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}


// Deliveries for applications with no connected peer must not occupy claim slots,
// or they starve every connected application once they exceed the batch limit.
func TestOfflineDeliveriesDoNotStarveConnectedApplications(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, offlineToken := newApp(t, ctx, admin, "starve-offline")
	offline := dial(t, ctx, h.URL, offlineToken)
	// Schedule them into the future, then disconnect well before they come due, so
	// they become pending deliveries that no replica is able to send.
	const stuck = 120
	for i := 0; i < stuck; i++ {
		write(t, ctx, offline, map[string]any{"op": "schedule.set", "idempotencyKey": fmt.Sprintf("stuck-%d", i), "key": fmt.Sprintf("stuck:%d", i), "event": "stuck.close", "after": "4s"})
		if got := read(t, ctx, offline); got["ok"] != true {
			t.Fatalf("stuck set %d: %#v", i, got)
		}
	}
	offline.CloseNow()
	time.Sleep(4600 * time.Millisecond)

	// This one is due now, so its attempt time is later than every stuck row and it
	// is the one a fixed claim limit would exclude.
	_, liveToken := newApp(t, ctx, admin, "starve-live")
	live := dial(t, ctx, h.URL, liveToken)
	defer live.CloseNow()
	write(t, ctx, live, map[string]any{"op": "schedule.set", "idempotencyKey": "live-set", "key": "live:one", "event": "live.close", "after": "1ms"})
	if got := read(t, ctx, live); got["ok"] != true {
		t.Fatalf("live set: %#v", got)
	}
	if got := delivery(t, ctx, live); got["event"] != "live.close" {
		t.Fatalf("live delivery event = %#v, want live.close", got["event"])
	}
}

// A start schedule that can never compute another occurrence must retire itself
// rather than occupy claim slots forever and starve the healthy ones behind it.
func TestUnresolvableStartSchedulesRetireInsteadOfStarving(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "start-starve")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()

	for _, name := range []string{"healthy", "doomed"} {
		write(t, ctx, admin, map[string]any{"op": "calendar.set", "applicationId": appID, "name": name, "data": map[string]any{"timezone": "UTC", "weekdays": []int{0, 1, 2, 3, 4, 5, 6}}})
		if got := read(t, ctx, admin); got["ok"] != true {
			t.Fatalf("calendar.set %s: %#v", name, got)
		}
	}
	// Separate workflows so the healthy count is independent of how many doomed rows
	// happened to be claimed in the window before the calendar was broken.
	for _, name := range []string{"starve", "healthy-run"} {
		definition := map[string]any{"initial": "emit", "states": map[string]any{
			"emit": map[string]any{"type": "emit", "target": appID, "event": name, "next": "end"},
			"end":  map[string]any{"type": "end"},
		}}
		write(t, ctx, admin, map[string]any{"op": "workflow.publish", "applicationId": appID, "name": name, "data": definition})
		if got := read(t, ctx, admin); got["ok"] != true {
			t.Fatalf("workflow.publish %s: %#v", name, got)
		}
	}

	clock := time.Now().UTC().Format("15:04")
	doomedAt := time.Now().UTC().AddDate(0, 0, -3).Truncate(time.Minute).Format(time.RFC3339)
	// The healthy schedule is due later, so under a fixed claim limit it is always
	// the one excluded once the doomed batch is larger than that limit.
	healthyAt := time.Now().UTC().AddDate(0, 0, -2).Truncate(time.Minute).Format(time.RFC3339)
	const doomed = 300
	for i := 0; i < doomed; i++ {
		write(t, ctx, admin, map[string]any{"op": "start-schedule.set", "applicationId": appID, "name": fmt.Sprintf("doomed-%d", i), "workflow": "starve", "calendar": "doomed", "localTime": clock, "missedPolicy": "skip", "at": doomedAt})
		if got := read(t, ctx, admin); got["ok"] != true {
			t.Fatalf("doomed set %d: %#v", i, got)
		}
	}
	write(t, ctx, admin, map[string]any{"op": "start-schedule.set", "applicationId": appID, "name": "healthy", "workflow": "healthy-run", "calendar": "healthy", "localTime": clock, "missedPolicy": "run_once_late", "at": healthyAt})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("healthy set: %#v", got)
	}

	// Make the doomed calendar permanently unresolvable: every date it can reach is ineligible.
	overrides := map[string]bool{}
	for offset := -30; offset < 370; offset++ {
		overrides[time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02")] = false
	}
	write(t, ctx, admin, map[string]any{"op": "calendar.set", "applicationId": appID, "name": "doomed", "data": map[string]any{"timezone": "UTC", "weekdays": []int{0, 1, 2, 3, 4, 5, 6}, "overrides": overrides}})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("calendar.set doomed: %#v", got)
	}

	if got := waitForWorkflowCount(t, ctx, admin, appID, "healthy-run", 1); got != 1 {
		t.Fatalf("healthy-run instances = %d, want 1", got)
	}
	broken := 0
	for _, row := range dashboardRows(t, ctx, admin, "start_schedules") {
		if row["status"] == "broken" {
			broken++
		}
	}
	if broken == 0 {
		t.Fatal("no start schedule retired after its calendar became unresolvable")
	}
}
