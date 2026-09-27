package server

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type chain struct {
	Key   string          `json:"key"`
	Event string          `json:"event"`
	After string          `json:"after"`
	Data  json.RawMessage `json:"data"`
}

func deadline(f frame) (time.Time, error) {
	if f.After != "" {
		duration, err := time.ParseDuration(f.After)
		return time.Now().UTC().Add(duration), err
	}
	if f.At != "" {
		return time.Parse(time.RFC3339, f.At)
	}
	return time.Time{}, errors.New("after or at required")
}

func parseChain(raw json.RawMessage) (chain, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return chain{}, nil
	}
	var next chain
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

func (s *Server) set(ctx context.Context, app string, f frame) reply {
	if f.Key == "" || f.Event == "" {
		return fail("key and event required")
	}
	if _, err := parseChain(f.Chain); err != nil {
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
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	var scheduleID string
	err = tx.QueryRow(ctx, `insert into schedules(id,application_id,schedule_key,event,payload,run_at,status,chain) values($1,$2,$3,$4,$5,$6,'scheduled',$7) on conflict(application_id,schedule_key) do update set event=excluded.event,payload=excluded.payload,run_at=excluded.run_at,status='scheduled',chain=excluded.chain,updated_at=now() returning id`, random(), app, f.Key, f.Event, payload, runAt, f.Chain).Scan(&scheduleID)
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
	if err = rememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
		return fail(err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err.Error())
	}
	return response
}

func (s *Server) cancel(ctx context.Context, app string, f frame) reply {
	if f.Key == "" {
		return fail("key required")
	}
	tx, err := s.db.Begin(ctx)
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
	if err = rememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
		return fail(err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err.Error())
	}
	return response
}

func (s *Server) extend(ctx context.Context, app string, f frame) reply {
	if f.Key == "" || f.By == "" {
		return fail("key and by required")
	}
	duration, err := time.ParseDuration(f.By)
	if err != nil {
		return fail(err.Error())
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	var runAt time.Time
	err = tx.QueryRow(ctx, `update schedules set run_at=greatest(run_at,now())+$3::interval,status='scheduled',updated_at=now() where application_id=$1 and schedule_key=$2 and status!='cancelled' returning run_at`, app, f.Key, duration.String()).Scan(&runAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail("schedule not found")
	}
	if err != nil {
		return fail(err.Error())
	}
	response := ok(map[string]any{"runAt": runAt})
	if err = rememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
		return fail(err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err.Error())
	}
	return response
}

func (s *Server) throttle(ctx context.Context, app string, f frame) reply {
	if f.Key == "" || f.Event == "" || f.Cooldown == "" {
		return fail("key, event and cooldown required")
	}
	duration, err := time.ParseDuration(f.Cooldown)
	if err != nil {
		return fail(err.Error())
	}
	if _, err = parseChain(f.Chain); err != nil {
		return fail(err.Error())
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	var allowed bool
	err = tx.QueryRow(ctx, `insert into throttles(application_id,throttle_key,until_at) values($1,$2,now()+$3::interval) on conflict(application_id,throttle_key) do update set until_at=excluded.until_at where throttles.until_at<=now() returning true`, app, f.Key, duration.String()).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		response := ok(map[string]bool{"triggered": false})
		if err = rememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
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
	if response := s.setTx(ctx, tx, app, f); !response.OK {
		return response
	}
	response := ok(map[string]bool{"triggered": true})
	if err = rememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
		return fail(err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err.Error())
	}
	return response
}

func (s *Server) setTx(ctx context.Context, tx pgx.Tx, app string, f frame) reply {
	payload := f.Data
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	var scheduleID string
	err := tx.QueryRow(ctx, `insert into schedules(id,application_id,schedule_key,event,payload,run_at,status,chain) values($1,$2,$3,$4,$5,now(),'scheduled',$6) on conflict(application_id,schedule_key) do update set event=excluded.event,payload=excluded.payload,run_at=excluded.run_at,status='scheduled',chain=excluded.chain,updated_at=now() returning id`, random(), app, f.Key, f.Event, payload, f.Chain).Scan(&scheduleID)
	if err != nil {
		return fail(err.Error())
	}
	if _, err = tx.Exec(ctx, `update deliveries set status='cancelled' where schedule_id=$1 and status='pending'`, scheduleID); err != nil {
		return fail(err.Error())
	}
	return ok(nil)
}
