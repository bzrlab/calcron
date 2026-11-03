package retention_test

import (
	"context"
	"encoding/json"
	"github.com/bzrlab/calcron/internal/server"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bzrlab/calcron"
	"github.com/bzrlab/calcron/internal/retention"
	"github.com/jackc/pgx/v5/pgxpool"
)

func database(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("CALCRON_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CALCRON_TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	// Each test has its own schema, including migrations and cleanup candidates.
	schema := "retention_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err = db.Exec(ctx, "create schema "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(ctx, "drop schema "+schema+" cascade") })
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	config, err := pgxpool.ParseConfig(parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	isolated, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(isolated.Close)
	if _, err = isolated.Exec(ctx, calcron.InitialMigration); err != nil {
		t.Fatal(err)
	}
	return ctx, isolated
}

func exec(t *testing.T, ctx context.Context, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupBoundsOldestFirstAndProtectsWork(t *testing.T) {
	ctx, db := database(t)
	exec(t, ctx, db, `insert into applications(id,namespace) values('app','app');
 insert into schedules(id,application_id,schedule_key,event,run_at,status,updated_at,awaiting_signal) values
 ('old','app','old','event',now(),'cancelled',now()-interval '60 days',false),
 ('newer','app','newer','event',now(),'cancelled',now()-interval '40 days',false),
 ('pending','app','pending','event',now(),'delivered',now()-interval '60 days',false),
 ('signal','app','signal','event',now(),'delivered',now()-interval '60 days',true),
 ('recent','app','recent','event',now(),'cancelled',now(),false);
 insert into deliveries(id,application_id,schedule_id,event,payload,status,next_attempt_at,created_at) values
 ('pending','app','pending','event','{}','pending',now(),now()-interval '60 days');`)
	counts, err := retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Schedules != 1 {
		t.Fatalf("counts: %+v", counts)
	}
	var ids []string
	rows, err := db.Query(ctx, `select id from schedules order by id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got := len(ids); got != 4 || ids[0] != "newer" {
		t.Fatalf("surviving schedules: %v", ids)
	}
}

func TestCleanupProtectsLeasesReferencesAndRecentCompletion(t *testing.T) {
	ctx, db := database(t)
	exec(t, ctx, db, `insert into applications(id,namespace) values('app','app');
 insert into schedules(id,application_id,schedule_key,event,run_at,status,updated_at) values
 ('parent','app','parent','event',now(),'delivered',now()-interval '60 days');
 insert into deliveries(id,application_id,schedule_id,event,payload,status,next_attempt_at,created_at,finished_at,locked_until) values
 ('acked','app',null,'event','{}','acked',now(),now()-interval '60 days',now()-interval '40 days',null),
 ('cancelled','app',null,'event','{}','cancelled',now(),now()-interval '60 days',now()-interval '40 days',null),
 ('leased','app','parent','event','{}','acked',now(),now()-interval '60 days',now()-interval '40 days',now()+interval '1 hour'),
 ('blocked','app',null,'event','{}','blocked',now(),now()-interval '60 days',null,null),
 ('recent','app',null,'event','{}','pending',now(),now()-interval '60 days',null,null);
 update deliveries set status='cancelled' where id='recent';
 insert into history(application_id,subject_type,subject_id,event,created_at) values
 ('app','delivery','acked','acked',now()-interval '40 days'),
 ('app','delivery','blocked','blocked',now()-interval '40 days'),
 ('app','schedule','parent','due',now()-interval '40 days'),
 ('app','workflow','workflow','completed',now()-interval '40 days');
 insert into idempotency(application_id,key,response,created_at) values
 ('app','old','{}',now()-interval '40 days'),('app','recent','{}',now());`)
	counts, err := retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Deliveries != 2 || counts.Schedules != 0 || counts.History != 1 || counts.Idempotency != 1 {
		t.Fatalf("counts: %+v", counts)
	}
	var survivors int
	if err := db.QueryRow(ctx, `select count(*) from deliveries where id in ('leased','blocked','recent')`).Scan(&survivors); err != nil {
		t.Fatal(err)
	}
	if survivors != 3 {
		t.Fatalf("protected deliveries: %d", survivors)
	}
}

func TestCleanupSkipsLockedRowsAndEventuallyRemovesReferences(t *testing.T) {
	ctx, db := database(t)
	exec(t, ctx, db, `insert into applications(id,namespace) values('app','app');
 insert into schedules(id,application_id,schedule_key,event,run_at,status,updated_at) values
 ('parent','app','parent','event',now(),'delivered',now()-interval '60 days');
 insert into deliveries(id,application_id,schedule_id,event,payload,status,next_attempt_at,finished_at) values
 ('old','app','parent','event','{}','acked',now(),now()-interval '60 days'),
 ('newer','app',null,'event','{}','acked',now(),now()-interval '40 days');`)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `select id from deliveries where id='old' for update`); err != nil {
		t.Fatal(err)
	}
	counts, err := retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Deliveries != 1 || counts.Schedules != 0 {
		t.Fatalf("locked row must preserve parent: %+v", counts)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	counts, err = retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Deliveries != 1 || counts.Schedules != 1 {
		t.Fatalf("released reference must expire: %+v", counts)
	}
}

func TestTickExpiresReplayCacheAndFinishedScheduleThroughCommands(t *testing.T) {
	ctx, db := database(t)
	s, err := server.New(ctx, server.Config{DatabaseURL: db.Config().ConnString(), AdminToken: "admin", Migration: calcron.InitialMigration, RetentionPeriod: time.Hour, RetentionInterval: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	admin, _, err := websocket.Dial(ctx, "ws"+h.URL[4:]+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.CloseNow()
	wsCommand := func(command any) map[string]any {
		t.Helper()
		if err := wsjson.Write(ctx, admin, command); err != nil {
			t.Fatal(err)
		}
		var response map[string]any
		if err := wsjson.Read(ctx, admin, &response); err != nil {
			t.Fatal(err)
		}
		if response["ok"] != true {
			t.Fatalf("command failed: %v", response)
		}
		return response
	}
	wsCommand(map[string]any{"op": "auth", "token": "admin"})
	app := wsCommand(map[string]any{"op": "app.create", "namespace": "tick", "name": "tick"})["data"].(map[string]any)
	command := func(body string) map[string]any {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, "POST", h.URL+"/commands", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+app["token"].(string))
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var got map[string]any
		if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["ok"] != true {
			t.Fatalf("HTTP command: %v", got)
		}
		return got
	}
	body := `{"op":"schedule.set","key":"tick","event":"tick","after":"24h","idempotencyKey":"set"}`
	first := command(body)
	command(`{"op":"schedule.cancel","key":"tick","idempotencyKey":"cancel"}`)
	exec(t, ctx, db, `update schedules set updated_at=now()-interval '2 hours'; update idempotency set created_at=now()-interval '2 hours'`)
	deadline := time.Now().Add(3 * time.Second)
	for {
		rows := wsCommand(map[string]any{"op": "dashboard.list", "name": "schedules"})["data"].([]any)
		if len(rows) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tick did not remove expired cancelled schedule")
		}
		time.Sleep(25 * time.Millisecond)
	}
	second := command(body)
	if reflect.DeepEqual(first["data"], second["data"]) {
		t.Fatalf("expired replay returned old schedule: %v", second)
	}
}

func TestReplayAndRecancelRestartRetentionWindow(t *testing.T) {
	ctx, db := database(t)
	exec(t, ctx, db, `insert into applications(id,namespace) values('app','app');
 insert into deliveries(id,application_id,event,payload,status,next_attempt_at,finished_at) values
 ('delivery','app','event','{}','cancelled',now(),now()-interval '60 days');
 update deliveries set status='pending' where id='delivery'`)
	var cleared bool
	if err := db.QueryRow(ctx, `select finished_at is null from deliveries where id='delivery'`).Scan(&cleared); err != nil {
		t.Fatal(err)
	}
	if !cleared {
		t.Fatal("replay kept terminal timestamp")
	}
	exec(t, ctx, db, `update deliveries set status='cancelled' where id='delivery'`)
	counts, err := retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Deliveries != 0 {
		t.Fatalf("recent recancellation expired: %+v", counts)
	}
	// Reapplying the startup migration must not reset the terminal age.
	exec(t, ctx, db, `update deliveries set finished_at=now()-interval '60 days' where id='delivery'`)
	exec(t, ctx, db, calcron.InitialMigration)
	counts, err = retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Deliveries != 1 {
		t.Fatalf("migration reset terminal age: %+v", counts)
	}
}

func TestHistoryAndReplayCacheBatchesAreOldestFirst(t *testing.T) {
	ctx, db := database(t)
	exec(t, ctx, db, `insert into applications(id,namespace) values('app','app');
 insert into history(application_id,subject_type,subject_id,event,created_at) values
 ('app','delivery','older','acked',now()-interval '60 days'),
 ('app','schedule','newer','cancelled',now()-interval '40 days');
 insert into idempotency(application_id,key,response,created_at) values
 ('app','older','{}',now()-interval '60 days'),('app','newer','{}',now()-interval '40 days')`)
	counts, err := retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	if counts.History != 1 || counts.Idempotency != 1 {
		t.Fatalf("counts: %+v", counts)
	}
	var subject, key string
	if err := db.QueryRow(ctx, `select subject_id from history`).Scan(&subject); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `select key from idempotency`).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if subject != "newer" || key != "newer" {
		t.Fatalf("wrong survivors: history=%s key=%s", subject, key)
	}
}

func TestMigrationGivesLegacyFinishedDeliveriesAFullWindow(t *testing.T) {
	ctx, db := database(t)
	exec(t, ctx, db, `drop trigger deliveries_finished_at on deliveries;
 alter table deliveries drop column finished_at;
 insert into applications(id,namespace) values('app','app');
 insert into deliveries(id,application_id,event,payload,status,next_attempt_at,created_at,acked_at) values
 ('legacy','app','event','{}','acked',now(),now()-interval '60 days',now()-interval '60 days');`)
	exec(t, ctx, db, calcron.InitialMigration)
	counts, err := retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Deliveries != 0 {
		t.Fatalf("legacy delivery lost its initial retention window: %+v", counts)
	}
}

func TestCleanupPreservesDeliveriesReferencedByUnfinishedWorkflow(t *testing.T) {
	ctx, db := database(t)
	exec(t, ctx, db, `insert into applications(id,namespace) values('app','app');
 insert into workflow_versions(application_id,name,version,definition) values('app','flow',1,'{}');
 insert into workflow_instances(id,application_id,workflow_name,workflow_version,current_state,status) values('instance','app','flow',1,'wait','waiting_signal');
 insert into deliveries(id,application_id,event,payload,status,next_attempt_at,finished_at,workflow_instance_id) values
 ('delivery','app','event','{}','acked',now(),now()-interval '60 days','instance');`)
	counts, err := retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Deliveries != 0 {
		t.Fatalf("unfinished workflow lost delivery: %+v", counts)
	}
	exec(t, ctx, db, `update workflow_instances set status='completed' where id='instance'`)
	counts, err = retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Deliveries != 1 {
		t.Fatalf("finished workflow delivery did not expire: %+v", counts)
	}
}

func TestCleanupTableLockDoesNotStarveOtherTables(t *testing.T) {
	ctx, db := database(t)
	exec(t, ctx, db, `insert into applications(id,namespace) values('app','app');
 insert into idempotency(application_id,key,response,created_at) values('app','old','{}',now()-interval '60 days')`)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `lock table deliveries in access exclusive mode`); err != nil {
		t.Fatal(err)
	}
	counts, err := retention.Cleanup(ctx, db, time.Now().Add(-30*24*time.Hour), 100)
	if err == nil {
		t.Fatal("expected delivery table lock failure")
	}
	if counts.Idempotency != 1 {
		t.Fatalf("unrelated cleanup starved: %+v, %v", counts, err)
	}
}

func TestKeylessEnqueueDoesNotStoreReplayResponses(t *testing.T) {
	ctx, db := database(t)
	config := db.Config().ConnConfig.ConnString()
	s, err := server.New(ctx, server.Config{DatabaseURL: config, AdminToken: "admin", Migration: calcron.InitialMigration})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	c, _, err := websocket.Dial(ctx, "ws"+h.URL[4:]+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	send := func(command any) map[string]any {
		t.Helper()
		if err := wsjson.Write(ctx, c, command); err != nil {
			t.Fatal(err)
		}
		var response map[string]any
		if err := wsjson.Read(ctx, c, &response); err != nil {
			t.Fatal(err)
		}
		if response["ok"] != true {
			t.Fatalf("command failed: %#v", response)
		}
		return response
	}
	send(map[string]any{"op": "auth", "token": "admin"})
	app := send(map[string]any{"op": "app.create", "namespace": "keyless", "name": "keyless"})["data"].(map[string]any)
	for i := 0; i < 2; i++ {
		body := strings.NewReader(`{"op":"enqueue","event":"job"}`)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL+"/commands", body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+app["token"].(string))
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		err = json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if err != nil || result["ok"] != true {
			t.Fatalf("enqueue: %#v %v", result, err)
		}
	}
	var cached, jobs int
	if err := db.QueryRow(ctx, `select (select count(*) from idempotency),(select count(*) from schedules)`).Scan(&cached, &jobs); err != nil {
		t.Fatal(err)
	}
	if cached != 0 || jobs != 2 {
		t.Fatalf("replay rows=%d jobs=%d", cached, jobs)
	}
}
