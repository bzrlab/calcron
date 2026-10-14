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

// recurrence is the clock rule stored on a direct Schedule. Every is a whole
// number of minutes aligned to UTC. Daily is a clock time in an IANA timezone.
type recurrence struct {
	Every    string `json:"every,omitempty"`
	Daily    string `json:"daily,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

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

func deadline(f protocol.Frame) (time.Time, []byte, error) {
	modes := 0
	if f.After != "" {
		modes++
	}
	if f.At != "" {
		modes++
	}
	if f.Every != "" {
		modes++
	}
	if f.Daily != "" {
		modes++
	}
	if f.Timezone != "" && f.Daily == "" {
		return time.Time{}, nil, errors.New("timezone requires daily")
	}
	if modes > 1 {
		return time.Time{}, nil, errors.New("exactly one of after, at, every, or daily")
	}
	if f.After != "" {
		duration, err := time.ParseDuration(f.After)
		return time.Now().UTC().Add(duration), nil, err
	}
	if f.At != "" {
		at, err := time.Parse(time.RFC3339, f.At)
		return at, nil, err
	}
	if f.Every != "" || f.Daily != "" {
		raw, err := json.Marshal(recurrence{Every: f.Every, Daily: f.Daily, Timezone: f.Timezone})
		if err != nil {
			return time.Time{}, nil, err
		}
		next, err := nextOccurrence(time.Now().UTC(), raw)
		return next, raw, err
	}
	return time.Time{}, nil, errors.New("after or at required")
}

func nextOccurrence(from time.Time, raw []byte) (time.Time, error) {
	var rule recurrence
	if err := json.Unmarshal(raw, &rule); err != nil {
		return time.Time{}, errors.New("invalid recurrence")
	}
	if rule.Every != "" {
		every, err := time.ParseDuration(rule.Every)
		if err != nil || every < time.Minute || every%time.Minute != 0 {
			return time.Time{}, errors.New("every must be whole minutes")
		}
		step := every.Nanoseconds()
		next := (from.UnixNano()/step + 1) * step
		return time.Unix(0, next).UTC(), nil
	}
	if rule.Daily != "" {
		return nextDaily(from, rule.Daily, rule.Timezone)
	}
	return time.Time{}, errors.New("invalid recurrence")
}

func nextDaily(from time.Time, clock, timezone string) (time.Time, error) {
	if timezone == "" {
		return time.Time{}, errors.New("timezone required")
	}
	if len(clock) != 5 || clock[2] != ':' {
		return time.Time{}, errors.New("invalid daily")
	}
	hour, hourErr := time.Parse("15:04", clock)
	if hourErr != nil {
		return time.Time{}, errors.New("invalid daily")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, errors.New("invalid timezone")
	}
	local := from.In(loc)
	y, m, d := local.Date()
	// ponytail: three days covers a DST gap. A zone that skips this clock all year needs a real calendar.
	for i := 0; i < 3; i++ {
		candidate := time.Date(y, m, d+i, hour.Hour(), hour.Minute(), 0, 0, loc)
		shown := candidate.In(loc)
		if candidate.After(from) && shown.Hour() == hour.Hour() && shown.Minute() == hour.Minute() {
			return candidate.UTC(), nil
		}
	}
	return time.Time{}, errors.New("invalid daily")
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
	runAt, recurrence, err := deadline(f)
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
	err = tx.QueryRow(ctx, `insert into schedules(id,application_id,schedule_key,event,payload,run_at,status,chain,recurrence) values($1,$2,$3,$4,$5,$6,'scheduled',$7,$8) on conflict(application_id,schedule_key) do update set event=excluded.event,payload=excluded.payload,run_at=excluded.run_at,status='scheduled',chain=excluded.chain,recurrence=excluded.recurrence,awaiting_signal=false,updated_at=now() returning id`, m.newID(), app, f.Key, f.Event, payload, runAt, f.Chain, recurrence).Scan(&scheduleID)
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
	err = tx.QueryRow(ctx, `insert into schedules(id,application_id,schedule_key,event,payload,run_at,status,chain,recurrence) values($1,$2,$3,$4,$5,now()+$7::interval,'scheduled',$6,null) on conflict(application_id,schedule_key) do update set event=excluded.event,payload=excluded.payload,run_at=excluded.run_at,status='scheduled',chain=excluded.chain,recurrence=null,awaiting_signal=false,updated_at=now() returning id`, m.newID(), app, f.Key, f.Event, payload, f.Chain, delay.String()).Scan(&scheduleID)
	if err != nil {
		return fail(err.Error())
	}
	if _, err = tx.Exec(ctx, `update deliveries set status='cancelled' where schedule_id=$1 and status='pending'`, scheduleID); err != nil {
		return fail(err.Error())
	}
	return ok(nil)
}

// AdvanceRecurrence arms the next clock occurrence on the same Schedule after
// its Delivery is acknowledged. A Schedule without a recurrence stays delivered.
func (m *Module) AdvanceRecurrence(ctx context.Context, tx pgx.Tx, scheduleID string) error {
	var runAt time.Time
	var raw []byte
	err := tx.QueryRow(ctx, `select run_at, recurrence from schedules where id=$1`, scheduleID).Scan(&runAt, &raw)
	if err != nil {
		return err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	next, err := nextOccurrence(runAt, raw)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `update schedules set run_at=$2, status='scheduled', awaiting_signal=true, updated_at=now() where id=$1`, scheduleID, next)
	return err
}

// SkipUnfinished records a due slot that must not start a Delivery while the
// previous occurrence is still awaiting its completion Signal, then arms the following slot.
func (m *Module) SkipUnfinished(ctx context.Context, tx pgx.Tx, scheduleID string) (bool, error) {
	var app string
	var runAt time.Time
	var raw []byte
	var awaiting bool
	err := tx.QueryRow(ctx, `select application_id, run_at, recurrence, awaiting_signal from schedules where id=$1`, scheduleID).Scan(&app, &runAt, &raw, &awaiting)
	if err != nil {
		return false, err
	}
	if !awaiting {
		return false, nil
	}
	slot, err := json.Marshal(map[string]string{"runAt": runAt.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event,data) values($1,'schedule',$2,'skipped',$3)`, app, scheduleID, slot); err != nil {
		return false, err
	}
	next, err := nextOccurrence(runAt, raw)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `update schedules set run_at=$2, status='scheduled', updated_at=now() where id=$1`, scheduleID, next)
	if err != nil {
		return false, err
	}
	return true, nil
}

// FinishOccurrence accepts the completion Signal for the open occurrence.
// The correlation key is the Schedule key. Acknowledgement does not call this.
func (m *Module) FinishOccurrence(ctx context.Context, tx pgx.Tx, app, event, key string) error {
	_, err := tx.Exec(ctx, `update schedules set awaiting_signal=false, updated_at=now() where application_id=$1 and schedule_key=$2 and event=$3 and awaiting_signal`, app, key, event)
	return err
}

func fail(err string) protocol.Reply { return protocol.Reply{Error: err} }
func ok(data any) protocol.Reply     { return protocol.Reply{OK: true, Data: data} }
