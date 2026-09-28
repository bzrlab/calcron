// Package dashboard owns read-only admin projections.
package dashboard

import (
	"context"
	"encoding/json"

	"github.com/bzrlab/calcron/internal/protocol"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Presence interface{ ConnectedApps() []string }
type Module struct {
	db       *pgxpool.Pool
	presence Presence
}

func New(db *pgxpool.Pool, presence Presence) *Module { return &Module{db: db, presence: presence} }

var queries = map[string]string{
	"apps":              `select id,namespace,created_at,id = any($1::text[]) as connected from applications order by created_at desc limit 100`,
	"schedules":         `select id,application_id,schedule_key,event,payload,chain,run_at,status,updated_at from schedules order by updated_at desc limit 100`,
	"deliveries":        `select id,schedule_id,workflow_instance_id,application_id,event,payload,status,attempts,next_attempt_at,last_sent_at,acked_at,created_at from deliveries order by created_at desc limit 100`,
	"workflows":         `select id,application_id,workflow_name,workflow_version,current_state,status,waiting_event,correlation_key,wake_at,input,state,updated_at from workflow_instances order by updated_at desc limit 100`,
	"history":           `select id,application_id,subject_type,subject_id,event,data,created_at from history where ($1::text = '' or subject_type = $1) and ($2::text = '' or subject_id = $2) and ($3::text = '' or application_id = $3) and ($4::bigint = 0 or id < $4) order by id desc limit 100`,
	"calendars":         `select application_id,name,definition,updated_at from calendars order by updated_at desc limit 100`,
	"tokens":            `select id,application_id,created_at,revoked_at from application_tokens order by created_at desc limit 100`,
	"start_schedules":   `select id,application_id,name,workflow_name,calendar_name,local_time,missed_policy,input,next_at,updated_at from start_schedules order by next_at limit 100`,
	"workflow_versions": `select distinct on (application_id,name) application_id,name,version,created_at from workflow_versions order by application_id,name,version desc limit 100`,
}

func (m *Module) List(ctx context.Context, f protocol.Frame) protocol.Reply {
	query, found := queries[f.Name]
	if !found {
		return fail("unknown dashboard list")
	}
	var args []any
	switch f.Name {
	case "apps":
		args = []any{m.presence.ConnectedApps()}
	case "history":
		args = []any{f.SubjectType, f.SubjectID, f.ApplicationID, f.Before}
	}
	var raw []byte
	if err := m.db.QueryRow(ctx, `select coalesce(json_agg(row_to_json(t)),'[]'::json) from (`+query+`) t`, args...).Scan(&raw); err != nil {
		return fail(err.Error())
	}
	return ok(json.RawMessage(raw))
}

func fail(err string) protocol.Reply { return protocol.Reply{Error: err} }
func ok(data any) protocol.Reply     { return protocol.Reply{OK: true, Data: data} }
