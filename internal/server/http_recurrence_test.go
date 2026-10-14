package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestHTTPEveryMinutesStaysOnTheClock(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-every")
	const secret = "every-secret"
	got := make(chan map[string]any, 2)
	hold := make(chan struct{})
	var releaseAck sync.Once
	release := func() { releaseAck.Do(func() { close(hold) }) }
	t.Cleanup(release)
	var holdFirst sync.Once
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("body: %v", err)
			return
		}
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil {
			t.Errorf("body %s", raw)
			return
		}
		got <- body
		holdFirst.Do(func() {
			<-hold
			time.Sleep(3 * time.Second)
		})
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(callback.Close)
	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": secret,
	})

	before := time.Now().UTC()
	set := mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "sweep:minutes", "event": "payment.reconcile",
		"every": "1m", "data": map[string]string{"sweep": "reconcile"},
		"chain": map[string]any{"key": "sweep:minutes:follow", "event": "payment.follow", "after": "1h"},
	})
	data := commandData(t, set)
	scheduleID, _ := data["scheduleId"].(string)
	runAt := parseInstant(t, data["runAt"])
	if scheduleID == "" || !runAt.After(before) || runAt.Sub(before) > time.Minute+2*time.Second || !onTheMinute(runAt) {
		t.Fatalf("first occurrence schedule %q runAt %v, declared at %v", scheduleID, runAt, before)
	}
	if rows := scheduleRows(t, ctx, admin, appID, "sweep:minutes"); len(rows) != 1 || rows[0]["id"] != scheduleID || rows[0]["status"] != "scheduled" {
		t.Fatalf("declaration did not leave one scheduled row: %#v", rows)
	}
	if len(scheduleRows(t, ctx, admin, appID, "sweep:minutes:follow")) != 0 {
		t.Fatal("chain existed before acknowledgement")
	}

	first := waitCallbackFor(t, got, 70*time.Second)
	if first["event"] != "payment.reconcile" || first["scheduleId"] != scheduleID || first["deliveryId"] == "" {
		t.Fatalf("delivery: %#v", first)
	}
	if rows := scheduleRows(t, ctx, admin, appID, "sweep:minutes"); len(rows) != 1 || rows[0]["status"] != "delivered" {
		t.Fatalf("occurrence did not stay on the declared Schedule: %#v", rows)
	}
	if len(scheduleRows(t, ctx, admin, appID, "sweep:minutes:follow")) != 0 {
		t.Fatal("chain started before acknowledgement")
	}
	release()

	acked := waitForApplicationDeliveryFor(t, ctx, admin, appID, first["deliveryId"].(string), "acked", 8*time.Second)
	ackedAt := parseInstant(t, acked["acked_at"])
	nextRows := scheduleRows(t, ctx, admin, appID, "sweep:minutes")
	if len(nextRows) != 1 || nextRows[0]["id"] != scheduleID || nextRows[0]["status"] != "scheduled" {
		t.Fatalf("next occurrence replaced the Schedule: %#v", nextRows)
	}
	nextAt := parseInstant(t, nextRows[0]["run_at"])
	if !nextAt.Equal(runAt.Add(time.Minute)) || !onTheMinute(nextAt) {
		t.Fatalf("next occurrence %v, want %v on the clock", nextAt, runAt.Add(time.Minute))
	}
	if nextAt.Sub(ackedAt.Add(time.Minute)).Abs() < time.Second {
		t.Fatalf("next occurrence %v drifted from acknowledgement %v", nextAt, ackedAt)
	}
	follow := scheduleRows(t, ctx, admin, appID, "sweep:minutes:follow")
	if len(follow) != 1 {
		t.Fatalf("chain after acknowledgement: %#v", follow)
	}
	followAt := parseInstant(t, follow[0]["run_at"])
	if time.Until(followAt) < 59*time.Minute || followAt.Equal(nextAt) {
		t.Fatalf("chain run_at = %v, want about one hour after acknowledgement", followAt)
	}
	if workflows := listRows(t, ctx, admin, map[string]any{"name": "workflows", "applicationId": appID}); len(workflows) != 0 {
		t.Fatalf("recurrence created a workflow instance: %#v", workflows)
	}
	if starts := listRows(t, ctx, admin, map[string]any{"name": "start_schedules", "applicationId": appID}); len(starts) != 0 {
		t.Fatalf("recurrence created a start schedule: %#v", starts)
	}
	mustCommand(t, h.URL, token, map[string]any{
		// The next slot starts only after the completion Signal. Acknowledgement armed the clock.
		"op": "signal", "idempotencyKey": "finished", "event": "payment.reconcile", "correlationKey": "sweep:minutes",
	})

	second := waitCallbackFor(t, got, 70*time.Second)
	if second["deliveryId"] == first["deliveryId"] || second["scheduleId"] != scheduleID {
		t.Fatalf("second occurrence: %#v", second)
	}
	waitForApplicationDelivery(t, ctx, admin, appID, second["deliveryId"].(string), "acked")
	again := scheduleRows(t, ctx, admin, appID, "sweep:minutes")
	if len(again) != 1 || again[0]["id"] != scheduleID {
		t.Fatalf("second occurrence added a Schedule row: %#v", again)
	}
	if got := parseInstant(t, again[0]["run_at"]); !got.Equal(runAt.Add(2 * time.Minute)) {
		t.Fatalf("second next occurrence %v, want %v", got, runAt.Add(2*time.Minute))
	}
	if len(scheduleRows(t, ctx, admin, appID, "sweep:minutes:follow")) != 1 {
		t.Fatal("chain acknowledgement added a Schedule row")
	}
}

func TestHTTPRecurrenceRejectsAClockThatIsNotASchedule(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-recur-reject")
	cases := []struct {
		name string
		body map[string]any
	}{
		{"seconds", map[string]any{"op": "schedule.set", "idempotencyKey": "seconds", "key": "sweep:seconds", "event": "payment.reconcile", "every": "90s"}},
		{"mixed", map[string]any{"op": "schedule.set", "idempotencyKey": "mixed", "key": "sweep:mixed", "event": "payment.reconcile", "every": "5m", "after": "1m"}},
		{"zone", map[string]any{"op": "schedule.set", "idempotencyKey": "zone", "key": "sweep:zone", "event": "tracking.maintain", "daily": "03:30"}},
		{"place", map[string]any{"op": "schedule.set", "idempotencyKey": "place", "key": "sweep:place", "event": "tracking.maintain", "daily": "03:30", "timezone": "Not/AZone"}},
	}
	for _, tc := range cases {
		status, got := postCommand(t, h.URL, token, tc.body)
		if status != http.StatusOK || got["ok"] == true || got["error"] == "" {
			t.Fatalf("%s accepted: status %d %#v", tc.name, status, got)
		}
	}
	if rows := listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID}); len(rows) != 0 {
		t.Fatalf("rejected recurrence created a Schedule: %#v", rows)
	}
}

func TestHTTPDailyClockUsesIANATimezone(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-daily")
	const secret = "daily-secret"
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		t.Fatal(err)
	}
	soon := time.Now().In(loc).Add(90 * time.Second)
	clock := fmt.Sprintf("%02d:%02d", soon.Hour(), soon.Minute())
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
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": secret,
	})

	declared := time.Now()
	set := mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "sweep:tracking", "event": "tracking.maintain",
		"daily": clock, "timezone": "Asia/Dhaka",
	})
	data := commandData(t, set)
	scheduleID, _ := data["scheduleId"].(string)
	runAt := parseInstant(t, data["runAt"])
	if scheduleID == "" || !sameClock(runAt, loc, soon.Hour(), soon.Minute()) || !runAt.After(declared) || runAt.Sub(declared) > 2*time.Minute {
		t.Fatalf("first occurrence %v in Asia/Dhaka, want %s after %v", runAt.In(loc), clock, declared)
	}
	if rows := scheduleRows(t, ctx, admin, appID, "sweep:tracking"); len(rows) != 1 || rows[0]["id"] != scheduleID {
		t.Fatalf("declaration rows: %#v", rows)
	}

	body := waitCallbackFor(t, got, 2*time.Minute)
	if body["event"] != "tracking.maintain" || body["scheduleId"] != scheduleID {
		t.Fatalf("delivery: %#v", body)
	}
	waitForApplicationDelivery(t, ctx, admin, appID, body["deliveryId"].(string), "acked")
	rows := scheduleRows(t, ctx, admin, appID, "sweep:tracking")
	if len(rows) != 1 || rows[0]["id"] != scheduleID || rows[0]["status"] != "scheduled" {
		t.Fatalf("next day replaced the Schedule: %#v", rows)
	}
	nextAt := parseInstant(t, rows[0]["run_at"])
	want := runAt.In(loc).AddDate(0, 0, 1)
	if !nextAt.Equal(want) || !sameClock(nextAt, loc, soon.Hour(), soon.Minute()) {
		t.Fatalf("next occurrence %v, want %v", nextAt.In(loc), want)
	}
	if workflows := listRows(t, ctx, admin, map[string]any{"name": "workflows", "applicationId": appID}); len(workflows) != 0 {
		t.Fatalf("daily recurrence created a workflow instance: %#v", workflows)
	}
}

func TestHTTPRecurrenceSkipsTheNextSlotUntilCompletionSignal(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-skip-unfinished")
	const key = "sweep:open"
	got := make(chan map[string]any, 4)
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
		"op": "callback.set", "idempotencyKey": "callback", "url": callback.URL, "secret": "skip-secret",
	})

	set := mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": key, "event": "payment.reconcile", "every": "1m",
	})
	data := commandData(t, set)
	scheduleID, _ := data["scheduleId"].(string)
	firstAt := parseInstant(t, data["runAt"])
	if scheduleID == "" {
		t.Fatal("schedule was not created")
	}

	first := waitCallbackFor(t, got, 70*time.Second)
	if first["event"] != "payment.reconcile" || first["scheduleId"] != scheduleID || first["deliveryId"] == "" {
		t.Fatalf("delivery: %#v", first)
	}
	deliveryID := first["deliveryId"].(string)
	acked := waitForApplicationDeliveryFor(t, ctx, admin, appID, deliveryID, "acked", 8*time.Second)
	if acked["status"] != "acked" {
		t.Fatalf("acknowledgement: %#v", acked)
	}
	armed := scheduleRows(t, ctx, admin, appID, key)
	if len(armed) != 1 || armed[0]["status"] != "scheduled" {
		t.Fatalf("acknowledgement did not keep the Schedule: %#v", armed)
	}
	nextAt := parseInstant(t, armed[0]["run_at"])
	if !nextAt.Equal(firstAt.Add(time.Minute)) {
		t.Fatalf("next slot %v, want %v", nextAt, firstAt.Add(time.Minute))
	}

	if wait := time.Until(nextAt.Add(time.Second)); wait > 0 {
		time.Sleep(wait)
	}
	deadline := time.Now().Add(5 * time.Second)
	var skipped map[string]any
	for time.Now().Before(deadline) {
		deliveries := listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID})
		if len(deliveries) != 1 || deliveries[0]["id"] != deliveryID || deliveries[0]["status"] != "acked" {
			t.Fatalf("acknowledgement started another delivery: %#v", deliveries)
		}
		for _, item := range listRows(t, ctx, admin, map[string]any{"name": "history", "applicationId": appID, "subjectType": "schedule", "subjectId": scheduleID}) {
			if item["event"] == "skipped" {
				skipped = item
			}
		}
		if skipped != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if skipped == nil {
		t.Fatal("skipped slot missing from history")
	}
	slot, _ := skipped["data"].(map[string]any)
	if slot == nil || !parseInstant(t, slot["runAt"]).Equal(nextAt) {
		t.Fatalf("skipped slot %#v, want %v", skipped["data"], nextAt)
	}
	following := scheduleRows(t, ctx, admin, appID, key)
	if len(following) != 1 || !parseInstant(t, following[0]["run_at"]).Equal(nextAt.Add(time.Minute)) {
		t.Fatalf("slot after the skip %#v, want %v", following, nextAt.Add(time.Minute))
	}
	select {
	case extra := <-got:
		t.Fatalf("delivery before completion signal: %#v", extra)
	default:
	}

	signaled := mustCommand(t, h.URL, token, map[string]any{
		"op": "signal", "idempotencyKey": "finished", "event": "payment.reconcile", "correlationKey": key,
	})
	if commandData(t, signaled)["matched"] != float64(0) {
		t.Fatalf("completion signal: %#v", signaled)
	}
	if rows := listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID}); len(rows) != 1 || rows[0]["status"] != "acked" {
		t.Fatalf("signal delivered the skipped slot: %#v", rows)
	}

	later := waitCallbackFor(t, got, 80*time.Second)
	if later["deliveryId"] == deliveryID || later["scheduleId"] != scheduleID || later["event"] != "payment.reconcile" {
		t.Fatalf("later occurrence: %#v", later)
	}
}

func TestHTTPMissedOccurrencesDeliverLatestOnce(t *testing.T) {
	ctx, h, admin := testServer(t, 0)
	appID, token := newApp(t, ctx, admin, "http-missed")
	const secret = "missed-secret"
	down := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	down.Close()

	var mu sync.Mutex
	var seen []string
	first := make(chan map[string]any, 1)
	hold := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(hold) }) }
	t.Cleanup(release)
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Errorf("body")
			return
		}
		id, _ := body["deliveryId"].(string)
		mu.Lock()
		seen = append(seen, id)
		n := len(seen)
		mu.Unlock()
		if n == 1 {
			first <- body
			<-hold
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(live.Close)

	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback", "url": down.URL, "secret": secret,
	})
	set := mustCommand(t, h.URL, token, map[string]any{
		"op": "schedule.set", "idempotencyKey": "arm", "key": "sweep:missed", "event": "payment.reconcile",
		"every": "1m",
	})
	data := commandData(t, set)
	scheduleID, _ := data["scheduleId"].(string)
	origin := parseInstant(t, data["runAt"])
	if scheduleID == "" || !onTheMinute(origin) {
		t.Fatalf("first occurrence %v", origin)
	}

	var pendingID string
	deadline := time.Now().Add(90 * time.Second)
	for pendingID == "" {
		for _, row := range listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID}) {
			if row["status"] == "pending" && row["schedule_id"] == scheduleID {
				pendingID, _ = row["id"].(string)
			}
		}
		if pendingID != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first occurrence created no Delivery")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// One later clock slot is enough: acknowledgement used to arm that slot
	// immediately and deliver it too. Wait it out, then require a single Delivery.
	readyAt := origin.Add(time.Minute + 2*time.Second)
	if wait := time.Until(readyAt); wait > 0 {
		time.Sleep(wait)
	}

	want := latestMinute(origin, time.Now().UTC())
	if !want.After(origin) {
		t.Fatalf("latest due %v is not past the first missed slot %v", want, origin)
	}
	rows := scheduleRows(t, ctx, admin, appID, "sweep:missed")
	if len(rows) != 1 || rows[0]["id"] != scheduleID || rows[0]["status"] != "delivered" {
		t.Fatalf("schedule while callback was unreachable: %#v", rows)
	}
	if got := parseInstant(t, rows[0]["run_at"]); !got.Equal(want) {
		t.Fatalf("due occurrence %v, want latest %v (first was %v)", got, want, origin)
	}
	if n := applicationDeliveryCount(t, ctx, admin, appID); n != 1 {
		t.Fatalf("missed slots created %d Deliveries", n)
	}
	still := waitForApplicationDelivery(t, ctx, admin, appID, pendingID, "pending")
	if still["attempts"] != float64(0) {
		t.Fatalf("unreachable callback changed the Delivery: %#v", still)
	}

	mustCommand(t, h.URL, token, map[string]any{
		"op": "callback.set", "idempotencyKey": "callback-live", "url": live.URL, "secret": secret,
	})
	body := waitCallbackFor(t, first, 5*time.Second)
	if body["deliveryId"] != pendingID || body["scheduleId"] != scheduleID || body["event"] != "payment.reconcile" {
		t.Fatalf("delivery when the callback answered: %#v", body)
	}
	answered := scheduleRows(t, ctx, admin, appID, "sweep:missed")
	if len(answered) != 1 || answered[0]["status"] != "delivered" || !parseInstant(t, answered[0]["run_at"]).Equal(want) {
		t.Fatalf("occurrence at delivery: %#v, want %v", answered, want)
	}
	release()

	waitForApplicationDelivery(t, ctx, admin, appID, pendingID, "acked")
	time.Sleep(1500 * time.Millisecond)
	mu.Lock()
	gotIDs := append([]string(nil), seen...)
	mu.Unlock()
	if len(gotIDs) != 1 || gotIDs[0] != pendingID {
		t.Fatalf("callback deliveries: %v", gotIDs)
	}
	if n := applicationDeliveryCount(t, ctx, admin, appID); n != 1 {
		t.Fatalf("catch-up created %d Deliveries", n)
	}
	nextRows := scheduleRows(t, ctx, admin, appID, "sweep:missed")
	if len(nextRows) != 1 || nextRows[0]["id"] != scheduleID || nextRows[0]["status"] != "scheduled" {
		t.Fatalf("next occurrence: %#v", nextRows)
	}
	nextAt := parseInstant(t, nextRows[0]["run_at"])
	if !nextAt.Equal(want.Add(time.Minute)) || !nextAt.After(time.Now()) {
		t.Fatalf("next occurrence %v, want %v still ahead", nextAt, want.Add(time.Minute))
	}
}

func latestMinute(origin, now time.Time) time.Time {
	if now.Before(origin) {
		return origin
	}
	steps := now.Sub(origin) / time.Minute
	return origin.Add(steps * time.Minute)
}

func applicationDeliveryCount(t *testing.T, ctx context.Context, admin *websocket.Conn, appID string) int {
	t.Helper()
	return len(listRows(t, ctx, admin, map[string]any{"name": "deliveries", "applicationId": appID}))
}

func sameClock(at time.Time, loc *time.Location, hour, minute int) bool {
	local := at.In(loc)
	return local.Hour() == hour && local.Minute() == minute && local.Second() == 0 && local.Nanosecond() == 0
}

func scheduleRows(t *testing.T, ctx context.Context, admin *websocket.Conn, appID, key string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	for _, row := range listRows(t, ctx, admin, map[string]any{"name": "schedules", "applicationId": appID}) {
		if row["schedule_key"] == key {
			rows = append(rows, row)
		}
	}
	return rows
}

func parseInstant(t *testing.T, value any) time.Time {
	t.Helper()
	text, _ := value.(string)
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		t.Fatalf("instant %v: %v", value, err)
	}
	return parsed
}

func onTheMinute(at time.Time) bool { return at.Equal(at.Truncate(time.Minute)) }

func waitForApplicationDeliveryFor(t *testing.T, ctx context.Context, admin *websocket.Conn, appID, id, status string, d time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(d)
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

func waitCallbackFor(t *testing.T, got <-chan map[string]any, d time.Duration) map[string]any {
	t.Helper()
	select {
	case body := <-got:
		return body
	case <-time.After(d):
		t.Fatal("callback received no delivery")
	}
	return nil
}
