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
