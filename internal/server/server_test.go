package server_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/calcron/calcron"
	"github.com/calcron/calcron/internal/server"
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
	ctx, h, admin := testServer(t, time.Millisecond)
	_, token := newApp(t, ctx, admin, "recovery")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "schedule.set", "idempotencyKey": "recovery-set", "key": "order:recovery", "event": "order.close", "after": "1ms"})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("set: %#v", got)
	}
	var first map[string]any
	for range 5 {
		got := delivery(t, ctx, app)
		if first == nil {
			first = got
		} else if got["deliveryId"] != first["deliveryId"] {
			t.Fatalf("retry changed delivery ID: first=%#v got=%#v", first, got)
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
	write(t, ctx, admin, map[string]any{"op": "dashboard.list", "name": name})
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

func dialUnauthorized(t *testing.T, ctx context.Context, rawURL, token string) {
	t.Helper()
	c, _, err := websocket.Dial(ctx, "ws"+rawURL[4:]+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	write(t, ctx, c, map[string]string{"op": "auth", "token": token})
	if got := read(t, ctx, c); got["error"] != "unauthorized" {
		t.Fatalf("revoked token authenticated: %#v", got)
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
		t.Fatalf("auth: %#v", got)
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
