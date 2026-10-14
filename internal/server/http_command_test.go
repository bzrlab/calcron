package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestWebSocketScheduleSetWithAfterAndAtUsesAfter(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, token := newApp(t, ctx, admin, "ws-after-at")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	fixed := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	before := time.Now().UTC()
	write(t, ctx, app, map[string]any{
		"op": "schedule.set", "idempotencyKey": "both", "key": "ticket:both", "event": "ticket.close",
		"after": "1h", "at": fixed,
	})
	got := read(t, ctx, app)
	if got["ok"] != true {
		t.Fatalf("after and at: %#v", got)
	}
	runAt := parseInstant(t, got["data"].(map[string]any)["runAt"])
	if runAt.Before(before.Add(59*time.Minute)) || runAt.After(before.Add(61*time.Minute)) {
		t.Fatalf("runAt %v, want about one hour after %v", runAt, before)
	}
}

func TestHTTPScheduleSetMatchesWebSocket(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, token := newApp(t, ctx, admin, "http-set")
	status, got := postCommand(t, h.URL, token, map[string]any{
		"id":             "set-http",
		"op":             "schedule.set",
		"idempotencyKey": "http-set-1",
		"key":            "ticket:http:close",
		"event":          "ticket.close",
		"after":          "1h",
		"data":           map[string]string{"ticketId": "http"},
	})
	if status != http.StatusOK || got["ok"] != true || got["id"] != "set-http" {
		t.Fatalf("http set: status %d %#v", status, got)
	}
	httpID := got["data"].(map[string]any)["scheduleId"].(string)

	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{
		"id":             "set-ws",
		"op":             "schedule.set",
		"idempotencyKey": "ws-set-1",
		"key":            "ticket:ws:close",
		"event":          "ticket.close",
		"after":          "1h",
		"data":           map[string]string{"ticketId": "ws"},
	})
	ws := read(t, ctx, app)
	if ws["ok"] != true || ws["id"] != "set-ws" {
		t.Fatalf("ws set: %#v", ws)
	}
	wsID := ws["data"].(map[string]any)["scheduleId"].(string)

	rows := dashboardRows(t, ctx, admin, "schedules")
	httpRow := rowWhere(t, rows, "id", httpID)
	wsRow := rowWhere(t, rows, "id", wsID)
	if httpRow["schedule_key"] != "ticket:http:close" || httpRow["event"] != "ticket.close" || httpRow["status"] != "scheduled" {
		t.Fatalf("http schedule: %#v", httpRow)
	}
	if wsRow["event"] != httpRow["event"] || wsRow["status"] != httpRow["status"] || wsRow["schedule_key"] != "ticket:ws:close" {
		t.Fatalf("ws schedule: %#v", wsRow)
	}
}

func TestHTTPScheduleCancelExtendAndThrottleMatchWebSocket(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-mutate")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()

	httpSet := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "http-arm", "key": "http:arm", "event": "ticket.arm", "after": "1h",
	}))
	write(t, ctx, app, map[string]any{"op": "schedule.set", "idempotencyKey": "ws-arm", "key": "ws:arm", "event": "ticket.arm", "after": "1h"})
	wsSet := commandData(t, read(t, ctx, app))

	httpCancel := mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.cancel", "idempotencyKey": "http-cancel", "key": "http:arm",
	})
	write(t, ctx, app, map[string]any{"op": "schedule.cancel", "idempotencyKey": "ws-cancel", "key": "ws:arm"})
	wsCancel := read(t, ctx, app)
	if httpCancel["data"].(map[string]any)["cancelled"] != true || wsCancel["data"].(map[string]any)["cancelled"] != true {
		t.Fatalf("cancel http %#v ws %#v", httpCancel["data"], wsCancel["data"])
	}
	rows := listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID})
	if rowWhere(t, rows, "id", httpSet["scheduleId"])["status"] != "cancelled" || rowWhere(t, rows, "id", wsSet["scheduleId"])["status"] != "cancelled" {
		t.Fatal("cancel did not match")
	}

	httpAgain := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "http-arm-2", "key": "http:later", "event": "ticket.later", "after": "1h",
	}))
	write(t, ctx, app, map[string]any{"op": "schedule.set", "idempotencyKey": "ws-arm-2", "key": "ws:later", "event": "ticket.later", "after": "1h"})
	wsAgain := commandData(t, read(t, ctx, app))
	httpExtended := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.extend", "idempotencyKey": "http-extend", "key": "http:later", "by": "1h",
	}))
	write(t, ctx, app, map[string]any{"op": "schedule.extend", "idempotencyKey": "ws-extend", "key": "ws:later", "by": "1h"})
	wsExtended := commandData(t, read(t, ctx, app))
	if !laterThan(t, httpExtended["runAt"], httpAgain["runAt"]) || !laterThan(t, wsExtended["runAt"], wsAgain["runAt"]) {
		t.Fatalf("extend http %#v ws %#v", httpExtended, wsExtended)
	}

	httpThrottle := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.throttle", "idempotencyKey": "http-throttle", "key": "http:notice", "event": "ticket.notice", "cooldown": "1h",
	}))
	write(t, ctx, app, map[string]any{"op": "schedule.throttle", "idempotencyKey": "ws-throttle", "key": "ws:notice", "event": "ticket.notice", "cooldown": "1h"})
	wsThrottle := commandData(t, readCommand(t, ctx, app))
	if httpThrottle["triggered"] != true || wsThrottle["triggered"] != true {
		t.Fatalf("throttle http %#v ws %#v", httpThrottle, wsThrottle)
	}
	httpAgainThrottle := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.throttle", "idempotencyKey": "http-throttle-2", "key": "http:notice", "event": "ticket.notice", "cooldown": "1h",
	}))
	write(t, ctx, app, map[string]any{"op": "schedule.throttle", "idempotencyKey": "ws-throttle-2", "key": "ws:notice", "event": "ticket.notice", "cooldown": "1h"})
	wsAgainThrottle := commandData(t, readCommand(t, ctx, app))
	if httpAgainThrottle["triggered"] != false || wsAgainThrottle["triggered"] != false {
		t.Fatalf("repeat throttle http %#v ws %#v", httpAgainThrottle, wsAgainThrottle)
	}
}

func TestHTTPWorkflowStartAndSignalMatchWebSocket(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-workflow")
	publish(t, ctx, admin, appID, "payment", map[string]any{"initial": "wait", "states": map[string]any{
		"wait": map[string]any{"type": "wait_signal", "event": "payment.confirmed", "correlationKeyExpr": "'order:' + input.orderId", "next": "end"},
		"end":  map[string]any{"type": "end"},
	}})
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()

	httpStart := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "workflow.start", "idempotencyKey": "http-start", "name": "payment", "data": map[string]string{"orderId": "http"},
	}))
	write(t, ctx, app, map[string]any{"op": "workflow.start", "name": "payment", "idempotencyKey": "ws-start", "data": map[string]string{"orderId": "ws"}})
	wsStart := commandData(t, read(t, ctx, app))
	rows := listRows(t, ctx, admin, map[string]any{"name": "workflows", "applicationId": appID})
	if rowWhere(t, rows, "id", httpStart["instanceId"])["status"] != "waiting_signal" || rowWhere(t, rows, "id", wsStart["instanceId"])["status"] != "waiting_signal" {
		t.Fatal("start did not wait")
	}

	httpSignal := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "signal", "idempotencyKey": "http-signal", "event": "payment.confirmed", "correlationKey": "order:http", "data": map[string]bool{"ok": true},
	}))
	write(t, ctx, app, map[string]any{"op": "signal", "event": "payment.confirmed", "correlationKey": "order:ws", "idempotencyKey": "ws-signal", "data": map[string]bool{"ok": true}})
	wsSignal := commandData(t, read(t, ctx, app))
	if httpSignal["matched"] != float64(1) || wsSignal["matched"] != float64(1) {
		t.Fatalf("signal http %#v ws %#v", httpSignal, wsSignal)
	}
	waitForWorkflowStatus(t, ctx, admin, httpStart["instanceId"].(string), "completed")
	waitForWorkflowStatus(t, ctx, admin, wsStart["instanceId"].(string), "completed")
}

func TestHTTPIdempotencyKeyReturnsOriginalResult(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-idem")
	missing, missingReply := postCommand(t, h.URL, token, map[string]any{"op": "schedule.set", "key": "missing", "event": "ticket.close", "after": "1h"})
	if missing != http.StatusOK || missingReply["error"] != "idempotencyKey required" {
		t.Fatalf("missing key: status %d error %v", missing, missingReply["error"])
	}

	first := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"id": "first", "op": "schedule.set", "idempotencyKey": "same", "key": "ticket:1", "event": "ticket.close", "after": "1h", "data": map[string]string{"ticketId": "1"},
	}))
	second := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"id": "second", "op": "schedule.set", "idempotencyKey": "same", "key": "ticket:2", "event": "ticket.other", "after": "2h", "data": map[string]string{"ticketId": "2"},
	}))
	if first["scheduleId"] != second["scheduleId"] || first["runAt"] != second["runAt"] {
		t.Fatalf("reused key changed result: %#v %#v", first, second)
	}
	rows := listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID})
	if len(rows) != 1 || rows[0]["schedule_key"] != "ticket:1" || rows[0]["event"] != "ticket.close" {
		t.Fatalf("reused key applied a second schedule: %#v", rows)
	}

	publish(t, ctx, admin, appID, "payment", map[string]any{"initial": "wait", "states": map[string]any{
		"wait": map[string]any{"type": "wait_signal", "event": "payment.confirmed", "correlationKeyExpr": "'order:' + input.orderId", "next": "end"},
		"end":  map[string]any{"type": "end"},
	}})
	firstInstance := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "workflow.start", "idempotencyKey": "start-1", "name": "payment", "data": map[string]string{"orderId": "1"},
	}))
	secondInstance := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "workflow.start", "idempotencyKey": "start-2", "name": "payment", "data": map[string]string{"orderId": "2"},
	}))
	signaled := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "signal", "idempotencyKey": "pay", "event": "payment.confirmed", "correlationKey": "order:1",
	}))
	replayed := commandData(t, mustCommand(t, h.URL, token, map[string]any{
		"op": "signal", "idempotencyKey": "pay", "event": "payment.confirmed", "correlationKey": "order:2", "data": map[string]bool{"ok": false},
	}))
	if signaled["matched"] != float64(1) || replayed["matched"] != signaled["matched"] {
		t.Fatalf("reused signal key: %#v %#v", signaled, replayed)
	}
	if row := rowWhere(t, listRows(t, ctx, admin, map[string]any{"name": "workflows", "applicationId": appID}), "id", secondInstance["instanceId"]); row["status"] != "waiting_signal" || row["correlation_key"] != "order:2" {
		t.Fatalf("reused signal key advanced the other instance: %#v", row)
	}
	waitForWorkflowStatus(t, ctx, admin, firstInstance["instanceId"].(string), "completed")
}

func TestHTTPRotatedTokenWorksUntilRevoked(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, original := newApp(t, ctx, admin, "http-tokens")
	write(t, ctx, admin, map[string]any{"op": "app.token.rotate", "applicationId": appID})
	rotated := read(t, ctx, admin)
	if rotated["ok"] != true {
		t.Fatal("rotate failed")
	}
	replacement := rotated["data"].(map[string]any)["token"].(string)
	tokenID := rotated["data"].(map[string]any)["tokenId"].(string)

	if _, got := postCommand(t, h.URL, replacement, map[string]any{
		"op": "schedule.set", "idempotencyKey": "rotated", "key": "rotated", "event": "ticket.rotated", "after": "1h",
	}); got["ok"] != true {
		t.Fatal("rotated token rejected during overlap")
	}
	if _, got := postCommand(t, h.URL, original, map[string]any{
		"op": "schedule.set", "idempotencyKey": "original", "key": "original", "event": "ticket.original", "after": "1h",
	}); got["ok"] != true {
		t.Fatal("previous token rejected during overlap")
	}

	write(t, ctx, admin, map[string]any{"op": "app.token.revoke", "tokenId": tokenID})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatal("revoke failed")
	}
	status, got := postCommand(t, h.URL, replacement, map[string]any{
		"op": "schedule.set", "idempotencyKey": "revoked", "key": "revoked", "event": "ticket.revoked", "after": "1h",
	})
	if status != http.StatusUnauthorized || got["error"] != "unauthorized" || got["ok"] == true {
		t.Fatal("revoked token ran an HTTP command")
	}
	rows := listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID})
	for _, row := range rows {
		if row["schedule_key"] == "revoked" {
			t.Fatal("revoked token created a schedule")
		}
	}
	if _, still := postCommand(t, h.URL, original, map[string]any{
		"op": "schedule.set", "idempotencyKey": "original-still", "key": "original-still", "event": "ticket.still", "after": "1h",
	}); still["ok"] != true {
		t.Fatal("unrevoked token rejected")
	}
}

func TestHTTPCommandIgnoresTokenInURL(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-url")
	endpoint, err := url.Parse(h.URL + "/commands")
	if err != nil {
		t.Fatal(err)
	}
	endpoint.RawQuery = "token=" + url.QueryEscape(token)
	body, _ := json.Marshal(map[string]any{"op": "schedule.set", "idempotencyKey": "url", "key": "url", "event": "ticket.url", "after": "1h"})
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized || got["error"] != "unauthorized" {
		t.Fatal("token in URL authenticated an HTTP command")
	}
	for _, row := range listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID}) {
		if row["schedule_key"] == "url" {
			t.Fatal("token in URL created a schedule")
		}
	}
}

func TestHTTPAdminTokenCannotRunCommands(t *testing.T) {
	_, h, _ := testServer(t, 0)
	status, got := postCommand(t, h.URL, "test-admin", map[string]any{"op": "app.create", "name": "nope", "namespace": "http-admin-rejected"})
	if status != http.StatusUnauthorized || got["error"] != "unauthorized" || got["ok"] == true {
		t.Fatal("admin token ran an HTTP command")
	}
}

func mustCommand(t *testing.T, rawURL, token string, body map[string]any) map[string]any {
	t.Helper()
	status, got := postCommand(t, rawURL, token, body)
	if status != http.StatusOK || got["ok"] != true {
		t.Fatalf("%v: status %d error %v", body["op"], status, got["error"])
	}
	return got
}

func commandData(t *testing.T, got map[string]any) map[string]any {
	t.Helper()
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("reply data: error %v", got["error"])
	}
	return data
}

func readCommand(t *testing.T, ctx context.Context, c *websocket.Conn) map[string]any {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		got := read(t, ctx, c)
		if got["op"] != "delivery" {
			return got
		}
	}
	t.Fatal("no command reply")
	return nil
}

func laterThan(t *testing.T, got, earlier any) bool {
	t.Helper()
	left, leftErr := time.Parse(time.RFC3339Nano, got.(string))
	right, rightErr := time.Parse(time.RFC3339Nano, earlier.(string))
	if leftErr != nil || rightErr != nil {
		t.Fatalf("runAt %v %v", got, earlier)
	}
	return left.After(right)
}

func waitForWorkflowStatus(t *testing.T, ctx context.Context, admin *websocket.Conn, id, status string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, row := range dashboardRows(t, ctx, admin, "workflows") {
			if row["id"] == id && row["status"] == status {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("workflow %s did not become %s", id, status)
}

func postCommand(t *testing.T, rawURL, token string, body map[string]any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, rawURL+"/commands", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" && strings.Contains(req.URL.String(), token) {
		t.Fatal("token appears in URL")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("status %d body %s", res.StatusCode, raw)
	}
	return res.StatusCode, got
}
