package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/bzrlab/calcron/internal/server"
	"github.com/coder/websocket"
)

func TestPayloadLimitRejectsApplicationCommands(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, token := newApp(t, ctx, admin, "payload-limit")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	for _, op := range []string{"schedule.set", "schedule.throttle", "enqueue", "workflow.start", "signal", "delivery.ack", "schedule.cancel", "schedule.extend", "callback.set"} {
		t.Run(op, func(t *testing.T) {
			command := map[string]any{"id": "oversize", "op": op, "idempotencyKey": op, "data": strings.Repeat("x", 65535)}
			status, got := postCommand(t, h.URL, token, command)
			if status != http.StatusOK || got["error"] != "payload too large" || got["id"] != "oversize" {
				t.Fatalf("HTTP: %d %#v", status, got)
			}
			write(t, ctx, app, command)
			if got := readCommand(t, ctx, app); got["error"] != "payload too large" || got["id"] != "oversize" {
				t.Fatalf("WS: %#v", got)
			}
		})
	}
}

func TestPayloadLimitConfiguredBoundaryAndChain(t *testing.T) {
	ctx, h, admin := testServer(t, 0, func(c *server.Config) { c.MaxPayloadBytes = 128 })
	_, token := newApp(t, ctx, admin, "payload-boundary")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	for _, transport := range []string{"http", "ws"} {
		send := func(command map[string]any) map[string]any {
			if transport == "http" {
				_, got := postCommand(t, h.URL, token, command)
				return got
			}
			write(t, ctx, app, command)
			return readCommand(t, ctx, app)
		}
		// Quotes count toward the JSON byte size: 63 two-byte characters plus quotes = 128.
		command := map[string]any{"op": "schedule.set", "idempotencyKey": transport, "key": transport, "event": "boundary", "after": "1h", "data": strings.Repeat("é", 63)}
		if got := send(command); got["ok"] != true {
			t.Fatalf("boundary %s: %#v", transport, got)
		}
		command["data"] = strings.Repeat("é", 64)
		if got := send(command); got["error"] != "payload too large" {
			t.Fatalf("oversize retry %s: %#v", transport, got)
		}
		command["data"] = "retry"
		if got := send(command); got["ok"] != true {
			t.Fatalf("original response %s: %#v", transport, got)
		}
		command["idempotencyKey"] = transport + "chain"
		command["chain"] = map[string]any{"key": "child", "event": "child", "after": "1h", "data": strings.Repeat("x", 128)}
		if got := send(command); got["error"] != "payload too large" {
			t.Fatalf("chain %s: %#v", transport, got)
		}
	}
	for _, op := range []string{"workflow.publish", "start-schedule.set", "calendar.set"} {
		write(t, ctx, admin, map[string]any{"id": op, "op": op, "data": strings.Repeat("x", 127)})
		if got := read(t, ctx, admin); got["error"] != "payload too large" || got["id"] != op {
			t.Fatalf("admin %s: %#v", op, got)
		}
	}
}

func TestPayloadLimitBoundsTransportReads(t *testing.T) {
	ctx, h, admin := testServer(t, 0, func(c *server.Config) { c.MaxPayloadBytes = 128 })
	_, token := newApp(t, ctx, admin, "payload-transport")
	command := map[string]any{"op": "enqueue", "event": strings.Repeat("x", 65536+128)}
	status, got := postCommand(t, h.URL, token, command)
	if status != http.StatusRequestEntityTooLarge || got["error"] != "payload too large" {
		t.Fatalf("HTTP: %d %#v", status, got)
	}
	for _, authenticated := range []bool{false, true} {
		var app *websocket.Conn
		if authenticated {
			app = dial(t, ctx, h.URL, token)
		} else {
			var err error
			app, _, err = websocket.Dial(ctx, "ws"+h.URL[4:]+"/ws", nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		app.SetReadLimit(1 << 20)
		raw, _ := json.Marshal(command)
		_ = app.Write(ctx, websocket.MessageText, raw)
		_, _, err := app.Read(ctx)
		app.CloseNow()
		if websocket.CloseStatus(err) != websocket.StatusMessageTooBig || !strings.Contains(err.Error(), "payload too large") {
			t.Fatalf("WS authenticated=%v: %v", authenticated, err)
		}
	}
}

func TestPayloadLimitDefaultAcceptsLargeCommandsAndRejectsWithoutSideEffects(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	admin.SetReadLimit(1 << 20)
	appID, token := newApp(t, ctx, admin, "payload-default")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	for count, transport := range []string{"http", "ws"} {
		send := func(command map[string]any) map[string]any {
			if transport == "http" {
				_, got := postCommand(t, h.URL, token, command)
				return got
			}
			write(t, ctx, app, command)
			return readCommand(t, ctx, app)
		}
		command := map[string]any{"op": "schedule.set", "idempotencyKey": transport, "key": transport, "event": "large", "after": "1h", "data": strings.Repeat("x", 65535)}
		if got := send(command); got["error"] != "payload too large" {
			t.Fatalf("oversize: %#v", got)
		}
		if rows := listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID}); len(rows) != count {
			t.Fatalf("rejected command changed schedules: %d", len(rows))
		}
		command["data"] = strings.Repeat("x", 65534)
		if got := send(command); got["ok"] != true {
			t.Fatalf("exact default boundary %s: %#v", transport, got)
		}
	}
}

func TestPayloadLimitInvalidConfiguration(t *testing.T) {
	for _, limit := range []int64{-1, 1<<63 - 1} {
		_, err := server.New(context.Background(), server.Config{DatabaseURL: "postgres://unused", AdminToken: "unused", MaxPayloadBytes: limit})
		if err == nil || !strings.Contains(err.Error(), "MaxPayloadBytes") {
			t.Fatalf("limit %d: %v", limit, err)
		}
	}
}

func TestAuthenticationBudgetIsIndependentOfPayloadLimit(t *testing.T) {
	ctx, h, _ := testServer(t, 0, func(c *server.Config) { c.MaxPayloadBytes = 1 << 20 })
	c, _, err := websocket.Dial(ctx, "ws"+h.URL[4:]+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	write(t, ctx, c, map[string]any{"op": "auth", "token": strings.Repeat("x", 4096)})
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("oversized auth: %v", err)
	}
}
