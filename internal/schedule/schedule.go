package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bzrlab/calcron/internal/calendar"
	"github.com/bzrlab/calcron/internal/idempotency"
	"github.com/bzrlab/calcron/internal/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	db            *pgxpool.Pool
	calendars     *calendar.Module
	newID         func() string
	startWorkflow func(context.Context, string, protocol.Frame) protocol.Reply
}

type Chain struct {
	Key   string          `json:"key"`
	Event string          `json:"event"`
	After string          `json:"after"`
	Data  json.RawMessage `json:"data"`
}

func New(db *pgxpool.Pool, calendars *calendar.Module, newID func() string, startWorkflow func(context.Context, string, protocol.Frame) protocol.Reply) *Module {
	return &Module{db: db, calendars: calendars, newID: newID, startWorkflow: startWorkflow}
}

func deadline(f protocol.Frame) (time.Time, error) {
	if f.After != "" {
		duration, err := time.ParseDuration(f.After)
		return time.Now().UTC().Add(duration), err
	}
	if f.At != "" {
		return time.Parse(time.RFC3339, f.At)
	}
	return time.Time{}, errors.New("after or at required")
}

func ParseChain(raw json.RawMessage) (Chain, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return Chain{}, nil
	}
	var next Chain
	if err := json.Unmarshal(raw, &next); err != nil {
		return next, errors.New("invalid chain")
	}
	if next.Key == "" || next.Event == "" || next.After == "" {
		return next, errors.New("chain key, event and after required")
	}
	duration, err := time.ParseDuration(next.After)
	if err != nil || duration < 0 {
		return next, errors.New("invalid chain after")
	}
	return next, nil
}

func (m *Module) Set(ctx context.Context, app string, f protocol.Frame) protocol.Reply {
	if f.Key == "" || f.Event == "" {
		return fail("key and event required")
	}
	if _, err := ParseChain(f.Chain); err != nil {
		return fail(err.Error())
	}
	runAt, err := deadline(f)
	if err != nil {
		return fail(err.Error())
	}
	payload := f.Data
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	var scheduleID string
	err = tx.QueryRow(ctx, `insert into schedules(id,application_id,schedule_key,event,payload,run_at,status,chain) values($1,$2,$3,$4,$5,$6,'scheduled',$7) on conflict(application_id,schedule_key) do update set event=excluded.event,payload=excluded.payload,run_at=excluded.run_at,status='scheduled',chain=excluded.chain,updated_at=now() returning id`, m.newID(), app, f.Key, f.Event, payload, runAt, f.Chain).Scan(&scheduleID)
	if err != nil {
		return fail(err.Error())
	}
	if _, err = tx.Exec(ctx, `update deliveries set status='cancelled' where schedule_id=$1 and status='pending'`, scheduleID); err == nil {
		_, err = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event,data) values($2,'schedule',$1,'set',$3)`, scheduleID, app, payload)
	}
	if err != nil {
		return fail(err.Error())
	}
	response := ok(map[string]any{"scheduleId": scheduleID, "runAt": runAt})
	if err = idempotency.RememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
		return fail(err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err.Error())
	}
	return response
}

func (m *Module) Cancel(ctx context.Context, app string, f protocol.Frame) protocol.Reply {
	if f.Key == "" {
		return fail("key required")
	}
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	var scheduleID string
	err = tx.QueryRow(ctx, `update schedules set status='cancelled',updated_at=now() where application_id=$1 and schedule_key=$2 and status!='cancelled' returning id`, app, f.Key).Scan(&scheduleID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fail(err.Error())
	}
	if scheduleID != "" {
		if _, err = tx.Exec(ctx, `update deliveries set status='cancelled' where schedule_id=$1 and status='pending'`, scheduleID); err == nil {
			_, err = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event) values($2,'schedule',$1,'cancelled')`, scheduleID, app)
		}
		if err != nil {
			return fail(err.Error())
		}
	}
	response := ok(map[string]bool{"cancelled": scheduleID != ""})
	if err = idempotency.RememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
		return fail(err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err.Error())
	}
	return response
}

func (m *Module) Extend(ctx context.Context, app string, f protocol.Frame) protocol.Reply {
	if f.Key == "" || f.By == "" {
		return fail("key and by required")
	}
	duration, err := time.ParseDuration(f.By)
	if err != nil {
		return fail(err.Error())
	}
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	var scheduleID string
	var runAt time.Time
	err = tx.QueryRow(ctx, `update schedules set run_at=greatest(run_at,now())+$3::interval,status='scheduled',updated_at=now() where application_id=$1 and schedule_key=$2 and status!='cancelled' returning id,run_at`, app, f.Key, duration.String()).Scan(&scheduleID, &runAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail("schedule not found")
	}
	if err == nil {
		_, err = tx.Exec(ctx, `update deliveries set status='cancelled' where schedule_id=$1 and status='pending'`, scheduleID)
	}
	if err != nil {
		return fail(err.Error())
	}
	response := ok(map[string]any{"runAt": runAt})
	if err = idempotency.RememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
		return fail(err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err.Error())
	}
	return response
}

func (m *Module) Throttle(ctx context.Context, app string, f protocol.Frame) protocol.Reply {
	if f.Key == "" || f.Event == "" || f.Cooldown == "" {
		return fail("key, event and cooldown required")
	}
	duration, err := time.ParseDuration(f.Cooldown)
	if err != nil {
		return fail(err.Error())
	}
	if _, err = ParseChain(f.Chain); err != nil {
		return fail(err.Error())
	}
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	var allowed bool
	err = tx.QueryRow(ctx, `insert into throttles(application_id,throttle_key,until_at) values($1,$2,now()+$3::interval) on conflict(application_id,throttle_key) do update set until_at=excluded.until_at where throttles.until_at<=now() returning true`, app, f.Key, duration.String()).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		response := ok(map[string]bool{"triggered": false})
		if err = idempotency.RememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
			return fail(err.Error())
		}
		if err = tx.Commit(ctx); err != nil {
			return fail(err.Error())
		}
		return response
	}
	if err != nil {
		return fail(err.Error())
	}
	f.After = "0s"
	if response := m.SetAfterTx(ctx, tx, app, f); !response.OK {
		return response
	}
	response := ok(map[string]bool{"triggered": true})
	if err = idempotency.RememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
		return fail(err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err.Error())
	}
	return response
}

// SetAfterTx creates or replaces a schedule due f.After from the transaction's now().
func (m *Module) SetAfterTx(ctx context.Context, tx pgx.Tx, app string, f protocol.Frame) protocol.Reply {
	delay, err := time.ParseDuration(f.After)
	if err != nil {
		return fail("invalid after")
	}
	payload := f.Data
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	var scheduleID string
	err = tx.QueryRow(ctx, `insert into schedules(id,application_id,schedule_key,event,payload,run_at,status,chain) values($1,$2,$3,$4,$5,now()+$7::interval,'scheduled',$6) on conflict(application_id,schedule_key) do update set event=excluded.event,payload=excluded.payload,run_at=excluded.run_at,status='scheduled',chain=excluded.chain,updated_at=now() returning id`, m.newID(), app, f.Key, f.Event, payload, f.Chain, delay.String()).Scan(&scheduleID)
	if err != nil {
		return fail(err.Error())
	}
	if _, err = tx.Exec(ctx, `update deliveries set status='cancelled' where schedule_id=$1 and status='pending'`, scheduleID); err != nil {
		return fail(err.Error())
	}
	return ok(nil)
}

func fail(err string) protocol.Reply { return protocol.Reply{Error: err} }
func ok(data any) protocol.Reply     { return protocol.Reply{OK: true, Data: data} }
