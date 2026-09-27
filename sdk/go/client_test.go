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

type invoiceDueData struct {
	InvoiceID int `json:"invoiceId"`
}

var invoiceDue = EventOf[invoiceDueData]("invoice.due")

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

func TestConnectRejectsBadToken(t *testing.T) {
	h := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		if _, _, err = conn.Read(r.Context()); err == nil {
			_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"ok":false,"error":"bad token"}`))
		}
	})}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("loopback unavailable")
	}
	go h.Serve(l)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = Connect(ctx, "ws://"+l.Addr().String(), "invalid"); err == nil || err.Error() != "bad token" {
		t.Fatalf("Connect error = %v", err)
	}
}

func TestClientCommandsAndDelivery(t *testing.T) {
	acked := make(chan map[string]any, 1)
	h := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_, raw, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var auth map[string]any
		_ = json.Unmarshal(raw, &auth)
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"ok":true}`))
		for {
			_, raw, err = conn.Read(r.Context())
			if err != nil {
				return
			}
			var frame map[string]any
			_ = json.Unmarshal(raw, &frame)
			id := frame["id"]
			switch frame["op"] {
			case "schedule.set":
				if frame["key"] == "invalid-deadline" {
					_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"scheduleId":"invalid","runAt":"2026-09-28T10:00:00Z"}}`))
					continue
				}
				chain, valid := frame["chain"].(map[string]any)
				if !valid || chain["key"] != "invoice:42:overdue" || chain["after"] != "24h" {
					_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":false,"error":"invalid chain"}`))
					continue
				}
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"scheduleId":"s-1","runAt":"2026-09-28T10:00:00Z"}}`))
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"op":"delivery","deliveryId":"d-1","event":"invoice.due","data":{"invoiceId":42}}`))
			case "schedule.throttle":
				chain, valid := frame["chain"].(map[string]any)
				if !valid || chain["key"] != "invoice:42:overdue" || chain["after"] != "24h" {
					_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":false,"error":"invalid chain"}`))
					continue
				}
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"triggered":true}}`))
			case "schedule.cancel":
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"cancelled":true}}`))
			case "schedule.extend":
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"runAt":"2026-09-28T11:00:00Z"}}`))
			case "workflow.start":
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"instanceId":"w-1"}}`))
			case "signal":
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"matched":1}}`))
			case "delivery.ack":
				acked <- frame
				_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"id":"`+id.(string)+`","ok":true,"data":{"acked":true}}`))
			}
		}
	})}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("loopback unavailable")
	}
	go h.Serve(l)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, err := Connect(ctx, "ws://"+l.Addr().String(), "token")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	delivered := make(chan TypedDelivery[invoiceDueData], 1)
	OnTyped(c, invoiceDue, func(event TypedDelivery[invoiceDueData]) { delivered <- event })
	chain := &Chain{Key: "invoice:42:overdue", Event: "invoice.overdue", After: "24h", Data: map[string]any{"invoiceId": 42}}
	set, err := c.Set(ctx, Schedule{Key: "invoice:42", Event: "invoice.due", After: "1h", Chain: chain, IdempotencyKey: "set:invoice:42:v1"})
	if err != nil || set.ScheduleID != "s-1" {
		t.Fatalf("set = %#v, %v", set, err)
	}
	typed, err := SetTyped(ctx, c, TypedSchedule[invoiceDueData]{Key: "invoice:43", Event: invoiceDue, After: "1h", Data: invoiceDueData{InvoiceID: 43}, Chain: &TypedChain[invoiceDueData]{Key: "invoice:42:overdue", Event: invoiceDue, After: "24h"}, IdempotencyKey: "set:invoice:43:v1"})
	if err != nil || typed.ScheduleID != "s-1" {
		t.Fatalf("typed set = %#v, %v", typed, err)
	}
	throttled, err := c.Throttle(ctx, Throttle{Key: "invoice:42", Event: "invoice.due", Cooldown: "5m", Chain: chain, IdempotencyKey: "throttle:invoice:42:v1"})
	if err != nil || !throttled.Triggered {
		t.Fatalf("throttle = %#v, %v", throttled, err)
	}
	cancelled, err := c.Cancel(ctx, "invoice:42", "cancel:invoice:42:v1")
	if err != nil || !cancelled.Cancelled {
		t.Fatalf("cancel = %#v, %v", cancelled, err)
	}
	extended, err := c.Extend(ctx, "invoice:42", "1h", "extend:invoice:42:v1")
	if err != nil || extended.RunAt.IsZero() {
		t.Fatalf("extend = %#v, %v", extended, err)
	}
	workflow, err := c.Start(ctx, "invoice-lifecycle", "start:invoice:42:v1", map[string]any{"invoiceId": 42})
	if err != nil || workflow.InstanceID != "w-1" {
		t.Fatalf("start = %#v, %v", workflow, err)
	}
	signal, err := c.Signal(ctx, "invoice.paid", "invoice:42", "signal:invoice:42:v1", map[string]bool{"paid": true})
	if err != nil || signal.Matched != 1 {
		t.Fatalf("signal = %#v, %v", signal, err)
	}
	if _, err = c.Set(ctx, Schedule{Key: "invalid-deadline", Event: "invoice.due", IdempotencyKey: "invalid-deadline:v1"}); err == nil {
		t.Fatal("Set accepted a schedule without after or at")
	}
	select {
	case event := <-delivered:
		if event.Data.InvoiceID != 42 {
			t.Fatalf("delivery data = %#v", event.Data)
		}
		result, err := event.Ack(ctx)
		if err != nil || !result.Acked {
			t.Fatalf("ack = %#v, %v", result, err)
		}
	case <-ctx.Done():
		t.Fatal("delivery not received")
	}
	select {
	case frame := <-acked:
		if frame["idempotencyKey"] != "ack:d-1" {
			t.Fatalf("ack frame = %#v", frame)
		}
	case <-ctx.Done():
		t.Fatal("ack not sent")
	}
}
