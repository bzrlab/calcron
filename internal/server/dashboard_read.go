package server

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

type dashboardRead struct{ db *pgxpool.Pool }

func newDashboardRead(db *pgxpool.Pool) *dashboardRead { return &dashboardRead{db: db} }

func (r *dashboardRead) list(ctx context.Context, name string) reply {
	queries := map[string]string{
		"apps":       `select coalesce(json_agg(row_to_json(t)),'[]'::json) from (select id,namespace,created_at from applications order by created_at desc limit 100) t`,
		"schedules":  `select coalesce(json_agg(row_to_json(t)),'[]'::json) from (select id,application_id,schedule_key,event,run_at,status,updated_at from schedules order by updated_at desc limit 100) t`,
		"deliveries": `select coalesce(json_agg(row_to_json(t)),'[]'::json) from (select id,application_id,event,status,attempts,next_attempt_at,created_at from deliveries order by created_at desc limit 100) t`,
		"workflows":  `select coalesce(json_agg(row_to_json(t)),'[]'::json) from (select id,application_id,workflow_name,workflow_version,current_state,status,updated_at from workflow_instances order by updated_at desc limit 100) t`,
		"history":    `select coalesce(json_agg(row_to_json(t)),'[]'::json) from (select application_id,subject_type,subject_id,event,data,created_at from history order by id desc limit 100) t`,
		"calendars":  `select coalesce(json_agg(row_to_json(t)),'[]'::json) from (select application_id,name,definition,updated_at from calendars order by updated_at desc limit 100) t`,
	}
	q, found := queries[name]
	if !found {
		return fail("unknown dashboard list")
	}
	var raw []byte
	if err := r.db.QueryRow(ctx, q).Scan(&raw); err != nil {
		return fail(err.Error())
	}
	return ok(json.RawMessage(raw))
}
