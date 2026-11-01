package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	cron "github.com/bzrlab/calcron/sdk/go"
)

func TestEnqueueHTTPDeliversAndAcknowledges(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, token := newApp(t, ctx, admin, "enqueue-http")
	before := time.Now().UTC()
	status, got := postCommand(t, h.URL, token, map[string]any{
		"op": "enqueue", "event": "order.deliver", "data": map[string]any{"orderId": "42"}, "idempotencyKey": "deliver:42",
	})
	if status != http.StatusOK || got["ok"] != true {
		t.Fatalf("enqueue: %d %#v", status, got)
	}
	result := commandData(t, got)
	if result["scheduleId"] == "" || result["scheduleId"] == nil {
		t.Fatalf("missing schedule ID: %#v", result)
	}
	if runAt := parseInstant(t, result["runAt"]); runAt.Before(before) || runAt.After(time.Now().UTC()) {
		t.Fatalf("not immediate: %v", runAt)
	}
	// Retrying through the other transport returns the stored result, even with changed input.
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	write(t, ctx, app, map[string]any{"op": "enqueue", "idempotencyKey": "deliver:42", "event": "changed"})
	var retried, delivered map[string]any
	for retried == nil || delivered == nil {
		frame := read(t, ctx, app)
		if frame["op"] == "delivery" {
			delivered = frame
		} else {
			retried = frame
		}
	}
	if !reflect.DeepEqual(commandData(t, retried), result) {
		t.Fatalf("retry changed result: %#v != %#v", retried, got)
	}
	if delivered["event"] != "order.deliver" || !reflect.DeepEqual(delivered["data"], map[string]any{"orderId": "42"}) {
		t.Fatalf("delivery: %#v", delivered)
	}
	ack := mustCommand(t, h.URL, token, map[string]any{"op": "delivery.ack", "deliveryId": delivered["deliveryId"], "idempotencyKey": "ack:42"})
	if commandData(t, ack)["acked"] != true {
		t.Fatalf("ack: %#v", ack)
	}
}

func TestEnqueueGoSDK(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, token := newApp(t, ctx, admin, "enqueue-go")
	client, err := cron.Connect(ctx, "ws"+h.URL[4:]+"/ws", token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	deliveries := make(chan cron.TypedDelivery[map[string]string], 4)
	event := cron.EventOf[map[string]string]("order.deliver")
	cron.OnTyped(client, event, func(d cron.TypedDelivery[map[string]string]) { deliveries <- d })
	result, err := client.Enqueue(ctx, cron.Enqueue{Event: "order.deliver", Data: map[string]string{"orderId": "42"}, IdempotencyKey: "go:42"})
	if err != nil || result.ScheduleID == "" || result.RunAt.IsZero() {
		t.Fatalf("enqueue: %#v %v", result, err)
	}
	retry, err := cron.EnqueueTyped(ctx, client, cron.TypedEnqueue[map[string]string]{Event: event, Data: map[string]string{"orderId": "changed"}, IdempotencyKey: "go:42"})
	if err != nil || retry.ScheduleID != result.ScheduleID || !retry.RunAt.Equal(result.RunAt) {
		t.Fatalf("retry: %#v %v", retry, err)
	}
	select {
	case d := <-deliveries:
		if d.Data["orderId"] != "42" {
			t.Fatalf("payload: %#v", d.Data)
		}
		if ack, err := d.Ack(ctx); err != nil || !ack.Acked {
			t.Fatalf("ack: %#v %v", ack, err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no SDK delivery")
	}
	one, err := client.Enqueue(ctx, cron.Enqueue{Event: "order.deliver"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := cron.EnqueueTyped(ctx, client, cron.TypedEnqueue[map[string]string]{Event: event})
	if err != nil || one.ScheduleID == two.ScheduleID {
		t.Fatalf("keyless enqueue: %#v %#v %v", one, two, err)
	}
}

func TestEnqueueWebSocketGeneratesIndependentSchedules(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "enqueue-ws")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	ids := map[any]bool{}
	for i := 0; i < 2; i++ {
		write(t, ctx, app, map[string]any{"op": "enqueue", "event": "order.deliver", "data": map[string]string{"orderId": "42"}})
		result := commandData(t, readCommand(t, ctx, app))
		if result["scheduleId"] == nil || ids[result["scheduleId"]] {
			t.Fatalf("non-unique schedule: %#v", result)
		}
		ids[result["scheduleId"]] = true
	}
	rows := listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID})
	if len(rows) != 2 || rows[0]["schedule_key"] == rows[1]["schedule_key"] {
		t.Fatalf("keyless schedules: %#v", rows)
	}
}

func TestEnqueueIdempotencyIsConcurrentAndApplicationScoped(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "enqueue-concurrent")
	command := map[string]any{"op": "enqueue", "event": "order.deliver", "idempotencyKey": "shared"}
	type response struct {
		status int
		body   map[string]any
		err    error
	}
	responses := make(chan response, 8)
	// Use HTTP directly so failures are reported on the test goroutine.
	for i := 0; i < cap(responses); i++ {
		go func() {
			raw, _ := json.Marshal(command)
			req, _ := http.NewRequest(http.MethodPost, h.URL+"/commands", bytes.NewReader(raw))
			req.Header.Set("Authorization", "Bearer "+token)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				responses <- response{err: err}
				return
			}
			defer res.Body.Close()
			var body map[string]any
			err = json.NewDecoder(res.Body).Decode(&body)
			responses <- response{res.StatusCode, body, err}
		}()
	}
	var first map[string]any
	for i := 0; i < cap(responses); i++ {
		res := <-responses
		if res.err != nil || res.status != http.StatusOK || res.body["ok"] != true {
			t.Fatalf("concurrent enqueue: %#v", res)
		}
		if first == nil {
			first = commandData(t, res.body)
		} else if !reflect.DeepEqual(first, commandData(t, res.body)) {
			t.Fatalf("different results: %#v %#v", first, res.body)
		}
	}
	rows := listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID})
	if len(rows) != 1 {
		t.Fatalf("duplicate schedules: %#v", rows)
	}
	_, otherToken := newApp(t, ctx, admin, "enqueue-other")
	other := commandData(t, mustCommand(t, h.URL, otherToken, command))
	if other["scheduleId"] == first["scheduleId"] {
		t.Fatal("idempotency leaked across applications")
	}
}

func TestEnqueueUsesExistingRetryAndBlockRules(t *testing.T) {
	ctx, h, admin := testServer(t, time.Millisecond)
	_, token := newApp(t, ctx, admin, "enqueue-block")
	app := dial(t, ctx, h.URL, token)
	defer app.CloseNow()
	mustCommand(t, h.URL, token, map[string]any{"op": "enqueue", "event": "order.deliver"})
	first := delivery(t, ctx, app)
	for i := 1; i < 5; i++ {
		if retry := delivery(t, ctx, app); retry["deliveryId"] != first["deliveryId"] {
			t.Fatalf("retry changed ID: %#v", retry)
		}
	}
	waitForDeliveryStatus(t, ctx, admin, first["deliveryId"].(string), "blocked")
}

func TestEnqueueRequiresEventAndPreservesOtherCommandValidation(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	_, token := newApp(t, ctx, admin, "enqueue-validation")
	_, got := postCommand(t, h.URL, token, map[string]any{"op": "enqueue"})
	if got["ok"] != false || got["error"] != "event required" {
		t.Fatalf("missing event accepted: %#v", got)
	}
	_, got = postCommand(t, h.URL, token, map[string]any{"op": "schedule.set", "key": "ordinary", "event": "order.deliver", "after": "0s"})
	if got["error"] != "idempotencyKey required" {
		t.Fatalf("existing validation changed: %#v", got)
	}
}
