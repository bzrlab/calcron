package server_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	cron "github.com/bzrlab/calcron/sdk/go"
	"github.com/coder/websocket"
)

func cappedConnection(t *testing.T, ctx context.Context, url, token string, cap any) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.Dial(ctx, "ws"+url[4:]+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	write(t, ctx, c, map[string]any{"op": "auth", "token": token, "maxInFlight": cap})
	if got := read(t, ctx, c); got["ok"] != true {
		t.Fatalf("auth: %#v", got)
	}
	return c
}

func TestConnectionCapRetriesSameDeliveryAndResumesAfterHTTPAck(t *testing.T) {
	ctx, h, admin := testServer(t, 10*time.Millisecond)
	_, token := newApp(t, ctx, admin, "cap-retry")
	for i := 0; i < 3; i++ {
		mustCommand(t, h.URL, token, map[string]any{"op": "enqueue", "event": "job"})
	}
	c := cappedConnection(t, ctx, h.URL, token, 1)
	first := delivery(t, ctx, c)
	for i := 0; i < 2; i++ {
		if got := delivery(t, ctx, c); got["deliveryId"] != first["deliveryId"] {
			t.Fatalf("cap exceeded: first=%#v got=%#v", first, got)
		}
	}
	mustCommand(t, h.URL, token, map[string]any{"op": "delivery.ack", "deliveryId": first["deliveryId"], "idempotencyKey": "ack-first"})
	// Retries already written before the HTTP ack may still be buffered on the
	// socket. They precede the first newly eligible delivery.
	second := delivery(t, ctx, c)
	for second["deliveryId"] == first["deliveryId"] {
		second = delivery(t, ctx, c)
	}
	// A cached acknowledgement of the old delivery must not free the new slot.
	mustCommand(t, h.URL, token, map[string]any{"op": "delivery.ack", "deliveryId": first["deliveryId"], "idempotencyKey": "ack-first"})
	if got := delivery(t, ctx, c); got["deliveryId"] != second["deliveryId"] {
		t.Fatalf("cached ack released another slot: %#v", got)
	}
}

func TestConnectionCapsAreIndependentAndDisconnectReleasesOwnership(t *testing.T) {
	ctx, h, admin := testServer(t, time.Hour)
	_, token := newApp(t, ctx, admin, "cap-connections")
	one := cappedConnection(t, ctx, h.URL, token, 1)
	two := cappedConnection(t, ctx, h.URL, token, 1)
	for i := 0; i < 3; i++ {
		mustCommand(t, h.URL, token, map[string]any{"op": "enqueue", "event": "job"})
	}
	first, second := delivery(t, ctx, one), delivery(t, ctx, two)
	if first["deliveryId"] == second["deliveryId"] {
		t.Fatal("same delivery assigned twice")
	}
	time.Sleep(400 * time.Millisecond)
	for _, c := range []*websocket.Conn{one, two} {
		write(t, ctx, c, map[string]any{"op": "schedule.cancel", "key": "missing", "idempotencyKey": "barrier"})
		if got := read(t, ctx, c); got["op"] == "delivery" || got["ok"] != true {
			t.Fatalf("cap exceeded: %#v", got)
		}
	}
	one.CloseNow()
	replacement := cappedConnection(t, ctx, h.URL, token, 1)
	third := delivery(t, ctx, replacement)
	if third["deliveryId"] == first["deliveryId"] || third["deliveryId"] == second["deliveryId"] {
		t.Fatalf("retry delay changed: %#v", third)
	}
}

func TestConnectionCapBlockedDeliveryRetainsSlotUntilCancelled(t *testing.T) {
	ctx, h, admin := testServer(t, time.Millisecond)
	appID, token := newApp(t, ctx, admin, "cap-blocked")
	c := cappedConnection(t, ctx, h.URL, token, 1)
	mustCommand(t, h.URL, token, map[string]any{"op": "enqueue", "event": "job"})
	first := delivery(t, ctx, c)
	for i := 1; i < 5; i++ {
		if got := delivery(t, ctx, c); got["deliveryId"] != first["deliveryId"] {
			t.Fatalf("retry: %#v", got)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		rows := listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID, "status": "blocked"})
		if len(rows) == 1 && rows[0]["id"] == first["deliveryId"] {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("delivery did not become blocked")
		}
		time.Sleep(25 * time.Millisecond)
	}
	mustCommand(t, h.URL, token, map[string]any{"op": "enqueue", "event": "job"})
	time.Sleep(400 * time.Millisecond)
	write(t, ctx, c, map[string]any{"op": "schedule.cancel", "key": "missing", "idempotencyKey": "barrier"})
	if got := read(t, ctx, c); got["ok"] != true {
		t.Fatalf("blocked slot released: %#v", got)
	}
	write(t, ctx, admin, map[string]any{"op": "delivery.cancel", "deliveryId": first["deliveryId"]})
	if got := read(t, ctx, admin); got["ok"] != true {
		t.Fatalf("cancel: %#v", got)
	}
	if got := delivery(t, ctx, c); got["deliveryId"] == first["deliveryId"] {
		t.Fatalf("cancelled delivery sent: %#v", got)
	}
}

func TestConnectionCapValidation(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, token := newApp(t, ctx, admin, "cap-validation")
	c, _, err := websocket.Dial(ctx, "ws"+h.URL[4:]+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	write(t, ctx, c, map[string]any{"op": "auth", "token": token, "maxInFlight": -1})
	if got := read(t, ctx, c); got["ok"] != false || got["error"] != "maxInFlight must be a non-negative integer" {
		t.Fatalf("negative cap: %#v", got)
	}
}

func TestConnectionCapGoSDKResumesAfterAck(t *testing.T) {
	ctx, h, admin := testServer(t, time.Hour)
	_, token := newApp(t, ctx, admin, "cap-sdk")
	c, err := cron.Connect(ctx, "ws"+h.URL[4:]+"/ws", token, cron.ConnectOptions{MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	received := make(chan cron.Event, 4)
	c.On("job", func(e cron.Event) { received <- e })
	for i := 0; i < 2; i++ {
		mustCommand(t, h.URL, token, map[string]any{"op": "enqueue", "event": "job"})
	}
	var first cron.Event
	select {
	case first = <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("no first delivery")
	}
	select {
	case e := <-received:
		t.Fatalf("cap exceeded: %s", e.ID)
	case <-time.After(400 * time.Millisecond):
	}
	if ack, err := first.Ack(ctx); err != nil || !ack.Acked {
		t.Fatalf("ack: %#v %v", ack, err)
	}
	select {
	case e := <-received:
		if e.ID == first.ID {
			t.Fatal("acked delivery retried")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ack did not free slot")
	}
}

func TestConnectionCapDisconnectAllowsRetryOnReplacement(t *testing.T) {
	ctx, h, admin := testServer(t, 10*time.Millisecond)
	_, token := newApp(t, ctx, admin, "cap-replacement")
	c := cappedConnection(t, ctx, h.URL, token, 1)
	mustCommand(t, h.URL, token, map[string]any{"op": "enqueue", "event": "job"})
	first := delivery(t, ctx, c)
	c.CloseNow()
	replacement := cappedConnection(t, ctx, h.URL, token, 1)
	if got := delivery(t, ctx, replacement); got["deliveryId"] != first["deliveryId"] {
		t.Fatalf("replacement lost pending delivery: %#v", got)
	}
}

func TestConnectionCapDefaultsToUnlimited(t *testing.T) {
	for _, explicitZero := range []bool{false, true} {
		t.Run(fmt.Sprint(explicitZero), func(t *testing.T) {
			ctx, h, admin := testServer(t, time.Hour)
			_, token := newApp(t, ctx, admin, "cap-unlimited")
			var c *websocket.Conn
			if explicitZero {
				c = cappedConnection(t, ctx, h.URL, token, 0)
			} else {
				c = dial(t, ctx, h.URL, token)
				defer c.CloseNow()
			}
			for i := 0; i < 4; i++ {
				mustCommand(t, h.URL, token, map[string]any{"op": "enqueue", "event": "job"})
			}
			ids := map[any]bool{}
			for i := 0; i < 4; i++ {
				got := delivery(t, ctx, c)
				if ids[got["deliveryId"]] {
					t.Fatalf("duplicate: %#v", got)
				}
				ids[got["deliveryId"]] = true
			}
		})
	}
}
