package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
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

func TestHTTPRefusedCallbackCountsAttemptsAndBlocks(t *testing.T) {
	retryBase := 20 * time.Millisecond
	ctx, h, admin := testServer(t, retryBase)
	appID, token := newApp(t, ctx, admin, "http-refuse")
	var mu sync.Mutex
	var ids []string
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Errorf("body")
			return
		}
		mu.Lock()
		ids = append(ids, body["deliveryId"].(string))
		mu.Unlock()
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(callback.Close)
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": "refuse-secret",
	})
	set := mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "ticket:refuse", "event": "ticket.refuse", "after": "1ms",
	})
	scheduleID := set["data"].(map[string]any)["scheduleId"].(string)

	var first string
	deadline := time.Now().Add(3 * time.Second)
	for first == "" {
		mu.Lock()
		if len(ids) > 0 {
			first = ids[0]
		}
		mu.Unlock()
		if first != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("callback received no delivery")
		}
		time.Sleep(25 * time.Millisecond)
	}
	counted := waitForApplicationAttempts(t, ctx, admin, appID, first, 1)
	if counted["status"] != "pending" {
		t.Fatalf("refused delivery: %#v", counted)
	}
	blocked := waitForApplicationDelivery(t, ctx, admin, appID, first, "blocked")
	if blocked["attempts"] != float64(5) {
		t.Fatalf("blocked delivery: %#v", blocked)
	}
	mu.Lock()
	got := append([]string(nil), ids...)
	mu.Unlock()
	if len(got) < 5 {
		t.Fatalf("offers before block = %d, want at least 5: %v", len(got), got)
	}
	for _, id := range got {
		if id != first {
			t.Fatalf("retry changed delivery id: %v", got)
		}
	}
	waitForQuietOffers(t, &mu, &ids, len(got))
	listed := listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID, "status": "blocked"})
	if len(listed) != 1 || listed[0]["id"] != first || listed[0]["event"] != "ticket.refuse" {
		t.Fatalf("delivery list: %#v", listed)
	}
	var due bool
	for _, item := range listRows(t, ctx, admin, map[string]any{"name": "history", "applicationId": appID, "subjectType": "schedule", "subjectId": scheduleID}) {
		if item["event"] == "due" && item["subject_id"] == scheduleID {
			due = true
		}
	}
	if !due {
		t.Fatal("blocked delivery missing schedule history")
	}

	mu.Lock()
	beforeReplay := len(ids)
	mu.Unlock()
	write(t, ctx, admin, map[string]any{"op": "delivery.replay", "deliveryId": first})
	if reply := read(t, ctx, admin); reply["ok"] != true || reply["data"].(map[string]any)["replayed"] != true {
		t.Fatalf("replay: %#v", reply)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		got := append([]string(nil), ids...)
		mu.Unlock()
		if len(got) > beforeReplay {
			if got[len(got)-1] != first {
				t.Fatalf("replay changed delivery id: %v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("replay did not offer the delivery")
		}
		time.Sleep(25 * time.Millisecond)
	}
	write(t, ctx, admin, map[string]any{"op": "delivery.cancel", "deliveryId": first})
	if reply := read(t, ctx, admin); reply["ok"] != true || reply["data"].(map[string]any)["cancelled"] != true {
		t.Fatalf("cancel: %#v", reply)
	}
	if got := waitForApplicationDelivery(t, ctx, admin, appID, first, "cancelled"); got["status"] != "cancelled" {
		t.Fatalf("cancelled delivery: %#v", got)
	}
}

func TestHTTPUnreachableCallbackStaysPendingAndRetriesSameID(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	t.Run("connection refused", func(t *testing.T) {
		down := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		down.Close()
		offerSameDeliveryAfterOutage(t, ctx, h, admin, down.URL)
	})
	t.Run("dns", func(t *testing.T) {
		offerSameDeliveryAfterOutage(t, ctx, h, admin, "http://calcron-offline.invalid/delivery")
	})
	t.Run("timeout", func(t *testing.T) {
		var mu sync.Mutex
		var calls int
		var first string
		again := make(chan string, 1)
		callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Errorf("body")
				return
			}
			id, _ := body["deliveryId"].(string)
			mu.Lock()
			calls++
			n := calls
			if n == 1 {
				first = id
			}
			mu.Unlock()
			if n == 1 {
				<-r.Context().Done()
				return
			}
			again <- id
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(callback.Close)
		appID, token := newApp(t, ctx, admin, "http-timeout")
		mustCommand(t, h.URL, token, map[string]any{
			"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": "timeout-secret",
		})
		mustCommand(t, h.URL, token, map[string]any{
			"op": "schedule.set", "idempotencyKey": "arm", "key": "ticket:timeout", "event": "ticket.timeout", "after": "1ms",
		})
		deadline := time.Now().Add(3 * time.Second)
		for {
			mu.Lock()
			id := first
			mu.Unlock()
			if id != "" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("callback received no delivery")
			}
			time.Sleep(25 * time.Millisecond)
		}
		mu.Lock()
		id := first
		mu.Unlock()
		started := time.Now()
		var offered string
		select {
		case offered = <-again:
		case <-time.After(15 * time.Second):
			t.Fatal("timed-out delivery was not offered again")
		}
		if waited := time.Since(started); waited < 8*time.Second {
			t.Fatalf("second offer after %s, want the client timeout", waited)
		}
		if offered != id {
			t.Fatalf("retry changed delivery id: got %s want %s", offered, id)
		}
		acked := waitForApplicationDelivery(t, ctx, admin, appID, id, "acked")
		if acked["attempts"] != float64(0) {
			t.Fatalf("timeout counted as a failed acknowledgement: %#v", acked)
		}
	})
}

func offerSameDeliveryAfterOutage(t *testing.T, ctx context.Context, h *httptest.Server, admin *websocket.Conn, offlineURL string) {
	t.Helper()
	appID, token := newApp(t, ctx, admin, strings.ReplaceAll(t.Name(), "/", "-"))
	got := make(chan string, 1)
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Errorf("body")
			return
		}
		select {
		case got <- body["deliveryId"].(string):
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(live.Close)
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": offlineURL, "secret": "offline-secret",
	})
	mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "ticket:offline", "event": "ticket.offline", "after": "1ms",
	})
	deadline := time.Now().Add(3 * time.Second)
	var pending map[string]any
	for pending == nil {
		for _, row := range listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID}) {
			if row["status"] == "pending" {
				pending = row
			}
		}
		if pending != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("offline delivery was not created")
		}
		time.Sleep(25 * time.Millisecond)
	}
	time.Sleep(800 * time.Millisecond)
	id := pending["id"].(string)
	row := waitForApplicationDelivery(t, ctx, admin, appID, id, "pending")
	if row["attempts"] != float64(0) {
		t.Fatalf("unreachable callback counted a failed acknowledgement: %#v", row)
	}
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback-live", "url": live.URL, "secret": "offline-secret",
	})
	var offered string
	select {
	case offered = <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("delivery was not offered when the callback answered")
	}
	if offered != id {
		t.Fatalf("retry changed delivery id: got %s want %s", offered, id)
	}
	acked := waitForApplicationDelivery(t, ctx, admin, appID, id, "acked")
	if acked["attempts"] != float64(0) {
		t.Fatalf("acked after outage: %#v", acked)
	}
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

func TestDueDeliveryOnLiveSocketSkipsCallback(t *testing.T) {
	ctx, h, admin := testServer(t, 50*time.Millisecond)
	appID, token := newApp(t, ctx, admin, "socket-wins")
	const secret = "socket-wins-secret"
	var mu sync.Mutex
	calls := 0
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(callback.Close)
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": secret,
	})
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "ticket:socket", "event": "ticket.socket", "after": "1ms",
	})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("set: %#v", got)
	}

	first, ok := nextDelivery(t, ctx, app, 4*time.Second)
	mu.Lock()
	n := calls
	mu.Unlock()
	if !ok {
		t.Fatalf("live socket delivery missing; callback calls=%d", n)
	}
	if first["event"] != "ticket.socket" || first["deliveryId"] == "" {
		t.Fatalf("live socket delivery: %#v", first)
	}
	second, ok := nextDelivery(t, ctx, app, 2*time.Second)
	if !ok || second["deliveryId"] != first["deliveryId"] {
		t.Fatalf("next socket attempt: %#v ok=%v", second, ok)
	}
	mu.Lock()
	n = calls
	mu.Unlock()
	if n != 0 {
		t.Fatalf("callback ran %d times while the socket was authenticated", n)
	}
	if row := waitForApplicationDelivery(t, ctx, admin, appID, first["deliveryId"].(string), "pending"); row["event"] != "ticket.socket" {
		t.Fatalf("pending delivery: %#v", row)
	}
}

func TestPendingDeliveryMovesToCallbackWhenSocketCloses(t *testing.T) {
	ctx, h, admin := testServer(t, 50*time.Millisecond)
	appID, token := newApp(t, ctx, admin, "socket-then-callback")
	const secret = "socket-then-callback-secret"
	entered := make(chan map[string]any, 1)
	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	var mu sync.Mutex
	calls := 0
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Errorf("body")
			return
		}
		mu.Lock()
		calls++
		mu.Unlock()
		select {
		case entered <- body:
		default:
		}
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(callback.Close)
	t.Cleanup(free)
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": secret,
	})
	app := dial(t, ctx, h.URL, token)
	write(t, ctx, app, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "ticket:handoff", "event": "ticket.handoff", "after": "1ms",
	})
	if got := read(t, ctx, app); got["ok"] != true {
		t.Fatalf("set: %#v", got)
	}
	first := delivery(t, ctx, app)
	if first["deliveryId"] == "" {
		t.Fatalf("live socket delivery: %#v", first)
	}
	mu.Lock()
	if calls != 0 {
		mu.Unlock()
		t.Fatal("callback ran while the socket was authenticated")
	}
	mu.Unlock()
	app.CloseNow()

	var body map[string]any
	select {
	case body = <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("callback received no delivery after the socket closed")
	}
	if body["deliveryId"] != first["deliveryId"] || body["event"] != "ticket.handoff" {
		t.Fatalf("callback delivery: %#v", body)
	}
	again := dial(t, ctx, h.URL, token)
	defer again.CloseNow()
	if extra, ok := nextDelivery(t, ctx, again, 400*time.Millisecond); ok {
		t.Fatalf("in-flight callback attempt also offered on the socket: first %v extra %#v", first["deliveryId"], extra)
	}
	mu.Lock()
	n := calls
	mu.Unlock()
	if n != 1 {
		t.Fatalf("callback attempts = %d, want 1", n)
	}
	free()
	waitForApplicationDelivery(t, ctx, admin, appID, first["deliveryId"].(string), "acked")
}

func TestHTTPEmptyCallbackSecretReleasesTheDeliveryLease(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-lease")
	got := make(chan map[string]any, 1)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("body")
			return
		}
		got <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(callback.Close)
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": "lease-secret",
	})
	conn, err := pgx.Connect(ctx, os.Getenv("CALCRON_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	if _, err = conn.Exec(ctx, `update applications set callback_secret='' where id=$1`, appID); err != nil {
		t.Fatal(err)
	}
	mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "ticket:lease", "event": "ticket.close", "after": "1ms",
	})
	select {
	case body := <-got:
		t.Fatalf("delivered with an empty secret: %#v", body)
	case <-time.After(time.Second):
	}
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback-again", "url": callback.URL, "secret": "lease-secret",
	})
	select {
	case body := <-got:
		if body["event"] != "ticket.close" || body["deliveryId"] == "" {
			t.Fatalf("delivery: %#v", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("delivery stayed leased after the callback secret was missing")
	}
}

func nextDelivery(t *testing.T, ctx context.Context, c *websocket.Conn, wait time.Duration) (map[string]any, bool) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		rctx, cancel := context.WithTimeout(ctx, time.Until(deadline))
		_, raw, err := c.Read(rctx)
		cancel()
		if err != nil {
			return nil, false
		}
		var msg map[string]any
		if json.Unmarshal(raw, &msg) != nil || msg["op"] != "delivery" {
			continue
		}
		return msg, true
	}
	return nil, false
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

func waitForQuietOffers(t *testing.T, mu *sync.Mutex, ids *[]string, already int) int {
	t.Helper()
	last := already
	steady := 0
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && steady < 2 {
		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		n := len(*ids)
		mu.Unlock()
		if n == last {
			steady++
			continue
		}
		steady = 0
		last = n
	}
	if steady < 2 {
		t.Fatalf("offers continued after the delivery blocked, last count %d", last)
	}
	return last
}

func waitForApplicationAttempts(t *testing.T, ctx context.Context, admin *websocket.Conn, appID, id string, attempts float64) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, row := range listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID}) {
			if row["id"] == id && row["attempts"] == attempts {
				return row
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("delivery %s did not reach %g attempts", id, attempts)
	return nil
}
