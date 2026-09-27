package server

import (
	"context"
	"time"
)

func (s *Server) setStartSchedule(ctx context.Context, f frame) reply {
	if f.ApplicationID == "" || f.Name == "" || f.Workflow == "" || f.Calendar == "" || f.LocalTime == "" {
		return fail("applicationId, name, workflow, calendar and localTime required")
	}
	if f.MissedPolicy != "skip" && f.MissedPolicy != "run_once_late" && f.MissedPolicy != "catch_up" {
		return fail("invalid missedPolicy")
	}
	definition, err := s.loadCalendar(ctx, f.ApplicationID, f.Calendar)
	if err != nil {
		return fail(err.Error())
	}
	next, err := nextCalendar(definition, time.Now().UTC(), f.LocalTime)
	if err != nil {
		return fail(err.Error())
	}
	if f.At != "" {
		next, err = time.Parse(time.RFC3339, f.At)
		if err != nil {
			return fail("invalid at")
		}
		valid, validErr := nextCalendar(definition, next.Add(-time.Nanosecond), f.LocalTime)
		if validErr != nil || !valid.Equal(next) {
			return fail("at must be an eligible calendar time")
		}
	}
	input := f.Data
	if len(input) == 0 {
		input = []byte(`{}`)
	}
	_, err = s.db.Exec(ctx, `insert into start_schedules(id,application_id,name,workflow_name,calendar_name,local_time,missed_policy,input,next_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9) on conflict(application_id,name) do update set workflow_name=excluded.workflow_name,calendar_name=excluded.calendar_name,local_time=excluded.local_time,missed_policy=excluded.missed_policy,input=excluded.input,next_at=excluded.next_at,updated_at=now()`, random(), f.ApplicationID, f.Name, f.Workflow, f.Calendar, f.LocalTime, f.MissedPolicy, input, next)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]any{"nextAt": next})
}

func (s *Server) startRecurring(ctx context.Context) {
	rows, err := s.db.Query(ctx, `select id,application_id,workflow_name,calendar_name,local_time,missed_policy,input,next_at from start_schedules where next_at<=now() order by next_at limit 100`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, app, workflow, calendar, clock, policy string
		var input []byte
		var due time.Time
		if rows.Scan(&id, &app, &workflow, &calendar, &clock, &policy, &input, &due) != nil {
			continue
		}
		definition, loadErr := s.loadCalendar(ctx, app, calendar)
		if loadErr != nil {
			continue
		}
		from := time.Now().UTC()
		if policy == "catch_up" {
			from = due.Add(time.Second)
		}
		next, nextErr := nextCalendar(definition, from, clock)
		if nextErr != nil {
			continue
		}
		tag, _ := s.db.Exec(ctx, `update start_schedules set next_at=$2,updated_at=now() where id=$1 and next_at=$3`, id, next, due)
		if tag.RowsAffected() != 1 || policy == "skip" {
			continue
		}
		s.startWorkflow(ctx, app, frame{Name: workflow, Data: input, IdempotencyKey: "start:" + id + ":" + due.Format(time.RFC3339Nano)})
	}
}
