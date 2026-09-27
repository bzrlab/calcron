package cron

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestClientReconnects(t *testing.T) {
	var connections atomic.Int32
	reconnected := make(chan struct{}, 1)
	h := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		_, _, err = c.Read(r.Context())
		if err != nil {
			return
		}
		b, _ := json.Marshal(map[string]any{"ok": true})
		_ = c.Write(r.Context(), websocket.MessageText, b)
		if connections.Add(1) == 1 {
			time.Sleep(50 * time.Millisecond)
			_ = c.Close(websocket.StatusGoingAway, "test")
			return
		}
		reconnected <- struct{}{}
		<-r.Context().Done()
	})}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("loopback unavailable")
	}
	go h.Serve(l)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, err := Connect(ctx, "ws://"+l.Addr().String(), "token")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	select {
	case <-reconnected:
	case <-ctx.Done():
		t.Fatal("client did not reconnect")
	}
}

func TestClientCommandsAndDelivery(t *testing.T) {
	acked := make(chan map[string]any, 1)
	h := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil { return }
		defer conn.CloseNow()
		_, raw, err := conn.Read(r.Context())
		if err != nil { return }
		var auth map[string]any
		_ = json.Unmarshal(raw, &auth)
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"ok":true}`))
		for {
			_, raw, err = conn.Read(r.Context())
			if err != nil { return }
			var frame map[string]any
			_ = json.Unmarshal(raw, &frame)
			id := frame["id"]
			switch frame["op"] {
			case "schedule.set":
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"scheduleId":"s-1","runAt":"2026-09-28T10:00:00Z"}}`))
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"op":"delivery","deliveryId":"d-1","event":"invoice.due","data":{"invoiceId":42}}`))
			case "schedule.throttle":
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"triggered":true}}`))
			case "delivery.ack":
				acked <- frame
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"acked":true}}`))
			}
		}
	})}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Skip("loopback unavailable") }
	go h.Serve(l)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, err := Connect(ctx, "ws://"+l.Addr().String(), "token")
	if err != nil { t.Fatal(err) }
	defer c.Close()
	delivered := make(chan Event, 1)
	c.On("invoice.due", func(event Event) { delivered <- event })
	set, err := c.Set(ctx, Schedule{Key: "invoice:42", Event: "invoice.due", After: "1h", IdempotencyKey: "set:invoice:42:v1"})
	if err != nil || set.ScheduleID != "s-1" { t.Fatalf("set = %#v, %v", set, err) }
	throttled, err := c.Throttle(ctx, Throttle{Key: "invoice:42", Event: "invoice.due", Cooldown: "5m", IdempotencyKey: "throttle:invoice:42:v1"})
	if err != nil || !throttled.Triggered { t.Fatalf("throttle = %#v, %v", throttled, err) }
	select {
	case event := <-delivered:
		result, err := event.Ack(ctx)
		if err != nil || !result.Acked { t.Fatalf("ack = %#v, %v", result, err) }
	case <-ctx.Done(): t.Fatal("delivery not received")
	}
	select {
	case frame := <-acked:
		if frame["idempotencyKey"] != "ack:d-1" { t.Fatalf("ack frame = %#v", frame) }
	case <-ctx.Done(): t.Fatal("ack not sent")
	}
}
