package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestHTTPOneShotScheduleDeliversAndAcknowledges(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-deliver")
	const secret = "callback-secret-not-the-token"
	got := make(chan map[string]any, 1)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Calcron-Callback-Secret") != secret {
			t.Errorf("callback secret = %q", r.Header.Get("Calcron-Callback-Secret"))
		}
		if r.Header.Get("Calcron-Callback-Secret") == token {
			t.Error("delivery presented the application token")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("body: %v", err)
			return
		}
		if strings.Contains(string(raw), secret) || strings.Contains(string(raw), token) {
			t.Errorf("secret or token leaked into body %s", raw)
		}
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil {
			t.Errorf("body %s", raw)
			return
		}
		got <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(callback.Close)

	registered := mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": secret,
	})
	reply, _ := json.Marshal(registered)
	if strings.Contains(string(reply), secret) {
		t.Fatalf("registration reply contains the callback secret: %s", reply)
	}
	mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "ticket:http", "event": "ticket.close", "after": "1ms",
		"data": map[string]string{"ticketId": "http"},
	})

	var body map[string]any
	select {
	case body = <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("callback received no delivery")
	}
	if body["event"] != "ticket.close" || body["deliveryId"] == "" {
		t.Fatalf("delivery: %#v", body)
	}
	data, _ := body["data"].(map[string]any)
	if data["ticketId"] != "http" {
		t.Fatalf("payload: %#v", body["data"])
	}
	for _, row := range dashboardRows(t, ctx, admin, "apps") {
		encoded, _ := json.Marshal(row)
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("dashboard exposed callback secret: %s", encoded)
		}
	}
	acked := waitForApplicationDelivery(t, ctx, admin, appID, body["deliveryId"].(string), "acked")
	if acked["event"] != "ticket.close" {
		t.Fatalf("acked delivery: %#v", acked)
	}
	encoded, _ := json.Marshal(acked)
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("dashboard delivery exposed callback secret: %s", encoded)
	}
}

func TestHTTPCallbackSecretStaysWithItsApplication(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appA, tokenA := newApp(t, ctx, admin, "http-secret-a")
	appB, tokenB := newApp(t, ctx, admin, "http-secret-b")
	const secretA, secretB = "secret-for-application-a", "secret-for-application-b"
	gotA := make(chan map[string]any, 1)
	gotB := make(chan map[string]any, 1)
	callbackA := httptest.NewServer(callbackHandler(t, secretA, tokenA, gotA))
	callbackB := httptest.NewServer(callbackHandler(t, secretB, tokenB, gotB))
	t.Cleanup(callbackA.Close)
	t.Cleanup(callbackB.Close)

	mustCommand(t, h.URL, tokenA, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback-a", "url": callbackA.URL, "secret": secretA,
	})
	mustCommand(t, h.URL, tokenB, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback-b", "url": callbackB.URL, "secret": secretB,
	})
	mustCommand(t, h.URL, tokenA, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm-a", "key": "ticket:a", "event": "ticket.a", "after": "1ms",
	})
	mustCommand(t, h.URL, tokenB, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm-b", "key": "ticket:b", "event": "ticket.b", "after": "1ms",
	})

	bodyA := waitCallback(t, gotA)
	bodyB := waitCallback(t, gotB)
	if bodyA["event"] != "ticket.a" || bodyB["event"] != "ticket.b" || bodyA["deliveryId"] == bodyB["deliveryId"] {
		t.Fatalf("crossed deliveries: %#v %#v", bodyA, bodyB)
	}
	ackedA := waitForApplicationDelivery(t, ctx, admin, appA, bodyA["deliveryId"].(string), "acked")
	ackedB := waitForApplicationDelivery(t, ctx, admin, appB, bodyB["deliveryId"].(string), "acked")
	if ackedA["application_id"] != appA || ackedB["application_id"] != appB {
		t.Fatalf("acked for wrong application: %#v %#v", ackedA, ackedB)
	}
}

func TestHTTPDeliveryDropLeavesPendingAndRetriesSameID(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-drop")
	var mu sync.Mutex
	var ids []string
	dropped := make(chan struct{})
	allow := make(chan struct{})
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Errorf("body")
			return
		}
		mu.Lock()
		ids = append(ids, body["deliveryId"].(string))
		n := len(ids)
		mu.Unlock()
		if n == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			conn.Close()
			close(dropped)
			return
		}
		<-allow
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(callback.Close)
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": "drop-secret",
	})
	mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "ticket:drop", "event": "ticket.drop", "after": "1ms",
	})

	select {
	case <-dropped:
	case <-time.After(3 * time.Second):
		t.Fatal("callback did not drop the first attempt")
	}
	mu.Lock()
	first := ids[0]
	mu.Unlock()
	pending := waitForApplicationDelivery(t, ctx, admin, appID, first, "pending")
	if pending["attempts"] != float64(0) {
		t.Fatalf("dropped attempt counted as failed: %#v", pending)
	}
	close(allow)

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(ids)
		same := n >= 2 && ids[1] == ids[0]
		mu.Unlock()
		if same {
			break
		}
		if n >= 2 {
			t.Fatalf("retry changed delivery id: %v", ids)
		}
		if time.Now().After(deadline) {
			t.Fatal("dropped delivery was not offered again")
		}
		time.Sleep(25 * time.Millisecond)
	}
	waitForApplicationDelivery(t, ctx, admin, appID, first, "acked")
}

func TestHTTPDeliveryAttemptDoesNotReachTwoReceivers(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-one")
	var mu sync.Mutex
	var inFlight, maxInFlight, requests int
	started := make(chan struct{})
	var once sync.Once
	release := make(chan struct{})
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		mu.Lock()
		inFlight++
		requests++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()
		once.Do(func() { close(started) })
		<-release
		mu.Lock()
		inFlight--
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(callback.Close)
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": "one-secret",
	})
	mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "ticket:one", "event": "ticket.one", "after": "1ms",
	})
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("callback received no delivery")
	}
	time.Sleep(800 * time.Millisecond)
	mu.Lock()
	gotMax, gotRequests := maxInFlight, requests
	mu.Unlock()
	if gotMax != 1 || gotRequests != 1 {
		t.Fatalf("in-flight attempts = %d, requests = %d", gotMax, gotRequests)
	}
	close(release)
	rows := listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID})
	if len(rows) != 1 {
		t.Fatalf("deliveries: %#v", rows)
	}
	waitForApplicationDelivery(t, ctx, admin, appID, rows[0]["id"].(string), "acked")
}

func callbackHandler(t *testing.T, secret, token string, got chan<- map[string]any) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Calcron-Callback-Secret") != secret || r.Header.Get("Calcron-Callback-Secret") == token {
			t.Errorf("secret header = %q", r.Header.Get("Calcron-Callback-Secret"))
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("body: %v", err)
			return
		}
		if strings.Contains(string(raw), secret) || strings.Contains(string(raw), token) {
			t.Errorf("secret or token leaked into body %s", raw)
		}
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil {
			t.Errorf("body %s", raw)
			return
		}
		got <- body
		w.WriteHeader(http.StatusNoContent)
	})
}

func waitCallback(t *testing.T, got <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case body := <-got:
		return body
	case <-time.After(3 * time.Second):
		t.Fatal("callback received no delivery")
	}
	return nil
}

func waitForApplicationDelivery(t *testing.T, ctx context.Context, admin *websocket.Conn, appID, id, status string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, row := range listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID}) {
			if row["id"] == id && row["status"] == status {
				return row
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("delivery %s did not become %s", id, status)
	return nil
}
