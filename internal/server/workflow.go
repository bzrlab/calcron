package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
)

type workflowDefinition struct {
	Initial string                   `json:"initial"`
	States  map[string]workflowState `json:"states"`
}
type workflowState struct {
	Type        string          `json:"type"`
	After       string          `json:"after"`
	Event       string          `json:"event"`
	Correlation string          `json:"correlationKey"`
	When        string          `json:"when"`
	True        string          `json:"true"`
	False       string          `json:"false"`
	Next        string          `json:"next"`
	Target      string          `json:"target"`
	Data        json.RawMessage `json:"data"`
	DataExpr    string          `json:"dataExpr"`
}
type calendarDefinition struct {
	Timezone  string          `json:"timezone"`
	Weekdays  []int           `json:"weekdays"`
	Overrides map[string]bool `json:"overrides"`
}

func parseWorkflow(raw json.RawMessage) (workflowDefinition, error) {
	var d workflowDefinition
	if json.Unmarshal(raw, &d) != nil || d.Initial == "" || d.States[d.Initial].Type == "" {
		return d, errors.New("invalid workflow definition")
	}
	for name, s := range d.States {
		if name == "" {
			return d, errors.New("empty workflow state")
		}
		switch s.Type {
		case "end":
		case "wait_time":
			if _, e := time.ParseDuration(s.After); e != nil || s.Next == "" {
				return d, errors.New("invalid wait_time")
			}
			if _, ok := d.States[s.Next]; !ok {
				return d, errors.New("workflow target not found")
			}
		case "wait_signal":
			if s.Event == "" || s.Correlation == "" || s.Next == "" {
				return d, errors.New("invalid wait_signal")
			}
			if _, ok := d.States[s.Next]; !ok {
				return d, errors.New("workflow target not found")
			}
		case "branch":
			if s.When == "" || s.True == "" || s.False == "" {
				return d, errors.New("invalid branch")
			}
			if _, e := compileCEL(s.When); e != nil {
				return d, e
			}
			if _, ok := d.States[s.True]; !ok {
				return d, errors.New("workflow target not found")
			}
			if _, ok := d.States[s.False]; !ok {
				return d, errors.New("workflow target not found")
			}
		case "emit":
			if s.Target == "" || s.Event == "" || s.Next == "" {
				return d, errors.New("invalid emit")
			}
			if s.DataExpr != "" {
				if _, e := compileCEL(s.DataExpr); e != nil {
					return d, e
				}
			}
			if _, ok := d.States[s.Next]; !ok {
				return d, errors.New("workflow target not found")
			}
		default:
			return d, errors.New("unknown workflow state")
		}
	}
	return d, nil
}
func compileCEL(expr string) (cel.Program, error) {
	env, e := cel.NewEnv(cel.Variable("input", cel.DynType), cel.Variable("state", cel.DynType))
	if e != nil {
		return nil, e
	}
	ast, iss := env.Compile(expr)
	if iss.Err() != nil {
		return nil, iss.Err()
	}
	return env.Program(ast)
}
func (s *Server) publishWorkflow(ctx context.Context, f frame) reply {
	if f.ApplicationID == "" || f.Name == "" {
		return fail("applicationId and name required")
	}
	d, e := parseWorkflow(f.Data)
	if e != nil {
		return fail(e.Error())
	}
	raw, _ := json.Marshal(d)
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return fail(e.Error())
	}
	defer tx.Rollback(ctx)
	var v int
	e = tx.QueryRow(ctx, `select coalesce(max(version),0)+1 from workflow_versions where application_id=$1 and name=$2`, f.ApplicationID, f.Name).Scan(&v)
	if e != nil {
		return fail(e.Error())
	}
	_, e = tx.Exec(ctx, `insert into workflow_versions(application_id,name,version,definition) values($1,$2,$3,$4)`, f.ApplicationID, f.Name, v, raw)
	if e != nil {
		return fail(e.Error())
	}
	if e = tx.Commit(ctx); e != nil {
		return fail(e.Error())
	}
	return ok(map[string]any{"version": v})
}
func (s *Server) startWorkflow(ctx context.Context, app string, f frame) reply {
	if f.Name == "" {
		return fail("name required")
	}
	var v int
	var raw []byte
	e := s.db.QueryRow(ctx, `select version,definition from workflow_versions where application_id=$1 and name=$2 order by version desc limit 1`, app, f.Name).Scan(&v, &raw)
	if e != nil {
		return fail("workflow not found")
	}
	d, e := parseWorkflow(raw)
	if e != nil {
		return fail(e.Error())
	}
	data := f.Data
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	id := random()
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return fail(e.Error())
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `insert into workflow_instances(id,application_id,workflow_name,workflow_version,input,current_state,status) values($1,$2,$3,$4,$5,$6,'running')`, id, app, f.Name, v, data, d.Initial)
	if e != nil {
		return fail(e.Error())
	}
	r := ok(map[string]string{"instanceId": id})
	if e = rememberTx(ctx, tx, app, f.IdempotencyKey, r); e != nil {
		return fail(e.Error())
	}
	if e = tx.Commit(ctx); e != nil {
		return fail(e.Error())
	}
	s.workflows.run(ctx, id)
	return r
}
func (s *Server) signal(ctx context.Context, app string, f frame) reply {
	if f.Event == "" || f.Correlation == "" {
		return fail("event and correlationKey required")
	}
	data := f.Data
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	rows, e := s.db.Query(ctx, `update workflow_instances set status='running',state=jsonb_set(jsonb_set(state,'{_signalState}',to_jsonb(current_state)),'{signal}',$4::jsonb),updated_at=now() where application_id=$1 and status='waiting_signal' and waiting_event=$2 and correlation_key=$3 returning id`, app, f.Event, f.Correlation, data)
	if e != nil {
		return fail(e.Error())
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	r := ok(map[string]int{"matched": len(ids)})
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return fail(e.Error())
	}
	defer tx.Rollback(ctx)
	if e = rememberTx(ctx, tx, app, f.IdempotencyKey, r); e != nil {
		return fail(e.Error())
	}
	if e = tx.Commit(ctx); e != nil {
		return fail(e.Error())
	}
	for _, id := range ids {
		s.workflows.run(ctx, id)
	}
	return r
}
func (e *workflowEngine) run(ctx context.Context, id string) {
	s := e.server
	for n := 0; n < 100; n++ {
		tx, e := s.db.Begin(ctx)
		if e != nil {
			return
		}
		var app, name, current, status string
		var version int
		var raw, input, stateRaw []byte
		e = tx.QueryRow(ctx, `select i.application_id,i.workflow_name,i.workflow_version,v.definition,i.input,i.state,i.current_state,i.status from workflow_instances i join workflow_versions v on (v.application_id=i.application_id and v.name=i.workflow_name and v.version=i.workflow_version) where i.id=$1 for update`, id).Scan(&app, &name, &version, &raw, &input, &stateRaw, &current, &status)
		if e != nil {
			tx.Rollback(ctx)
			return
		}
		d, e := parseWorkflow(raw)
		if e != nil {
			tx.Rollback(ctx)
			return
		}
		st, exists := d.States[current]
		if !exists {
			tx.Rollback(ctx)
			return
		}
		var in, state map[string]any
		_ = json.Unmarshal(input, &in)
		_ = json.Unmarshal(stateRaw, &state)
		if state == nil {
			state = map[string]any{}
		}
		next := ""
		stop := false
		switch st.Type {
		case "end":
			_, e = tx.Exec(ctx, `update workflow_instances set status='completed',updated_at=now() where id=$1`, id)
			stop = true
		case "wait_time":
			if state["_wakeState"] == current {
				delete(state, "_wakeState")
				next = st.Next
			} else {
				dly, _ := time.ParseDuration(st.After)
				state["_wakeState"] = current
				b, _ := json.Marshal(state)
				_, e = tx.Exec(ctx, `update workflow_instances set status='waiting_time',wake_at=now()+$2::interval,state=$3,updated_at=now() where id=$1`, id, dly.String(), b)
				stop = true
			}
		case "wait_signal":
			if state["_signalState"] == current {
				delete(state, "_signalState")
				next = st.Next
			} else {
				_, e = tx.Exec(ctx, `update workflow_instances set status='waiting_signal',waiting_event=$2,correlation_key=$3,updated_at=now() where id=$1`, id, st.Event, st.Correlation)
				stop = true
			}
		case "branch":
			p, ce := compileCEL(st.When)
			if ce != nil {
				e = ce
				break
			}
			out, _, ce := p.Eval(map[string]any{"input": in, "state": state})
			if ce != nil {
				e = ce
				break
			}
			b, good := out.Value().(bool)
			if !good {
				e = errors.New("CEL branch must return bool")
				break
			}
			if b {
				next = st.True
			} else {
				next = st.False
			}
		case "emit":
			data := st.Data
			if st.DataExpr != "" {
				p, ce := compileCEL(st.DataExpr)
				if ce != nil {
					e = ce
					break
				}
				out, _, ce := p.Eval(map[string]any{"input": in, "state": state})
				if ce != nil {
					e = ce
					break
				}
				data, _ = json.Marshal(out.Value())
			}
			if len(data) == 0 {
				data = []byte(`{}`)
			}
			_, e = tx.Exec(ctx, `insert into deliveries(id,application_id,event,payload,status,next_attempt_at,workflow_instance_id,next_state) values($1,$2,$3,$4,'pending',now(),$5,$6)`, random(), st.Target, st.Event, data, id, st.Next)
			if e == nil {
				_, e = tx.Exec(ctx, `update workflow_instances set status='waiting_ack',updated_at=now() where id=$1`, id)
			}
			stop = true
		}
		if e == nil && next != "" {
			_, e = tx.Exec(ctx, `update workflow_instances set current_state=$2,status='running',waiting_event=null,correlation_key=null,wake_at=null,state=$3,updated_at=now() where id=$1`, id, next, mustJSON(state))
		}
		if e == nil {
			_, e = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event) values($1,'workflow',$2,$3)`, app, id, st.Type)
		}
		if e != nil {
			tx.Rollback(ctx)
			return
		}
		if e = tx.Commit(ctx); e != nil {
			return
		}
		if stop {
			return
		}
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func (e *workflowEngine) wake(ctx context.Context) {
	s := e.server
	rows, err := s.db.Query(ctx, `update workflow_instances set status='running',updated_at=now() where id in (select id from workflow_instances where status='waiting_time' and wake_at<=now() limit 100 for update skip locked) returning id`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			e.run(ctx, id)
		}
	}
}
func (s *Server) setCalendar(ctx context.Context, f frame) reply {
	if f.ApplicationID == "" || f.Name == "" {
		return fail("applicationId and name required")
	}
	if _, e := parseCalendar(f.Data); e != nil {
		return fail(e.Error())
	}
	_, e := s.db.Exec(ctx, `insert into calendars(application_id,name,definition) values($1,$2,$3) on conflict(application_id,name) do update set definition=excluded.definition,updated_at=now()`, f.ApplicationID, f.Name, f.Data)
	if e != nil {
		return fail(e.Error())
	}
	return ok(nil)
}
func parseCalendar(raw []byte) (calendarDefinition, error) {
	var d calendarDefinition
	if json.Unmarshal(raw, &d) != nil || d.Timezone == "" {
		return d, errors.New("invalid calendar")
	}
	if _, e := time.LoadLocation(d.Timezone); e != nil {
		return d, errors.New("invalid timezone")
	}
	if len(d.Weekdays) == 0 {
		return d, errors.New("calendar needs weekdays")
	}
	return d, nil
}

func (s *Server) loadCalendar(ctx context.Context, app, name string) (calendarDefinition, error) {
	var raw []byte
	var d calendarDefinition
	if s.db.QueryRow(ctx, `select definition from calendars where application_id=$1 and name=$2`, app, name).Scan(&raw) != nil {
		return d, errors.New("calendar not found")
	}
	return d, json.Unmarshal(raw, &d)
}

// calendarOccurrences lists every eligible run in [at, until) for a stored calendar,
// or for an inline definition so editors can preview unsaved changes.
// A localTime that falls in a DST gap skips that date, as the Start schedule scheduler does.
func (s *Server) calendarOccurrences(ctx context.Context, f frame) reply {
	if f.LocalTime == "" || f.At == "" || f.Until == "" {
		return fail("localTime, at and until required")
	}
	var d calendarDefinition
	var e error
	switch {
	case f.Calendar != "" && f.ApplicationID == "":
		return fail("applicationId required with calendar")
	case f.Calendar != "":
		d, e = s.loadCalendar(ctx, f.ApplicationID, f.Calendar)
	case len(f.Data) > 0:
		d, e = parseCalendar(f.Data)
	default:
		return fail("calendar or data required")
	}
	if e != nil {
		return fail(e.Error())
	}
	loc, _ := time.LoadLocation(d.Timezone)
	from, e1 := time.Parse(time.RFC3339, f.At)
	until, e2 := time.Parse(time.RFC3339, f.Until)
	if e1 != nil || e2 != nil {
		return fail("invalid at or until")
	}
	if !until.After(from) || until.Sub(from) > 366*24*time.Hour {
		return fail("until must be after at and within 366 days")
	}
	out := []string{}
	for from = from.Add(-time.Nanosecond); ; {
		next, e := nextCalendar(d, from, f.LocalTime)
		if errors.Is(e, errLocalTimeGap) {
			l := from.In(loc)
			if from = time.Date(l.Year(), l.Month(), l.Day()+1, 0, 0, 0, 0, loc); !from.Before(until) {
				break
			}
			continue
		}
		if errors.Is(e, errNoEligibleDate) {
			break
		}
		if e != nil {
			return fail(e.Error())
		}
		if !next.Before(until) {
			break
		}
		out = append(out, next.Format(time.RFC3339))
		from = next
	}
	return ok(map[string]any{"occurrences": out})
}

var (
	errLocalTimeGap   = errors.New("localTime does not exist on calendar date")
	errNoEligibleDate = errors.New("no eligible calendar date")
)

func nextCalendar(d calendarDefinition, from time.Time, clock string) (time.Time, error) {
	loc, e := time.LoadLocation(d.Timezone)
	if e != nil {
		return time.Time{}, e
	}
	p := strings.Split(clock, ":")
	if len(p) != 2 {
		return time.Time{}, errors.New("localTime must be HH:MM")
	}
	h, e := strconv.Atoi(p[0])
	if e != nil {
		return time.Time{}, e
	}
	m, e := strconv.Atoi(p[1])
	if e != nil || h > 23 || m > 59 {
		return time.Time{}, errors.New("invalid localTime")
	}
	allowed := map[int]bool{}
	for _, v := range d.Weekdays {
		allowed[v] = true
	}
	local := from.In(loc)
	for i := 0; i < 370; i++ {
		day := local.AddDate(0, 0, i)
		date := day.Format("2006-01-02")
		yes := allowed[int(day.Weekday())]
		if x, ok := d.Overrides[date]; ok {
			yes = x
		}
		at := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc)
		if yes && (at.Year() != day.Year() || at.Month() != day.Month() || at.Day() != day.Day() || at.Hour() != h || at.Minute() != m) {
			return time.Time{}, errLocalTimeGap
		}
		if yes && at.After(from) {
			return at.UTC(), nil
		}
	}
	return time.Time{}, errNoEligibleDate
}
func (s *Server) nextCalendar(ctx context.Context, f frame) reply {
	if f.ApplicationID == "" || f.Calendar == "" || f.LocalTime == "" {
		return fail("applicationId, calendar and localTime required")
	}
	d, e := s.loadCalendar(ctx, f.ApplicationID, f.Calendar)
	if e != nil {
		return fail(e.Error())
	}
	from := time.Now().UTC()
	if f.At != "" {
		var e error
		from, e = time.Parse(time.RFC3339, f.At)
		if e != nil {
			return fail("invalid at")
		}
	}
	next, e := nextCalendar(d, from, f.LocalTime)
	if e != nil {
		return fail(e.Error())
	}
	return ok(map[string]any{"nextAt": next.Format(time.RFC3339)})
}
func (s *Server) setStartSchedule(ctx context.Context, f frame) reply {
	if f.ApplicationID == "" || f.Name == "" || f.Workflow == "" || f.Calendar == "" || f.LocalTime == "" {
		return fail("applicationId, name, workflow, calendar and localTime required")
	}
	if f.MissedPolicy != "skip" && f.MissedPolicy != "run_once_late" && f.MissedPolicy != "catch_up" {
		return fail("invalid missedPolicy")
	}
	d, e := s.loadCalendar(ctx, f.ApplicationID, f.Calendar)
	if e != nil {
		return fail(e.Error())
	}
	next, e := nextCalendar(d, time.Now().UTC(), f.LocalTime)
	if e != nil {
		return fail(e.Error())
	}
	if f.At != "" {
		next, e = time.Parse(time.RFC3339, f.At)
		if e != nil {
			return fail("invalid at")
		}
		valid, e := nextCalendar(d, next.Add(-time.Nanosecond), f.LocalTime)
		if e != nil || !valid.Equal(next) {
			return fail("at must be an eligible calendar time")
		}
	}
	data := f.Data
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	_, e = s.db.Exec(ctx, `insert into start_schedules(id,application_id,name,workflow_name,calendar_name,local_time,missed_policy,input,next_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9) on conflict(application_id,name) do update set workflow_name=excluded.workflow_name,calendar_name=excluded.calendar_name,local_time=excluded.local_time,missed_policy=excluded.missed_policy,input=excluded.input,next_at=excluded.next_at,updated_at=now()`, random(), f.ApplicationID, f.Name, f.Workflow, f.Calendar, f.LocalTime, f.MissedPolicy, data, next)
	if e != nil {
		return fail(e.Error())
	}
	return ok(map[string]any{"nextAt": next})
}
func (s *Server) startRecurring(ctx context.Context) {
	rows, e := s.db.Query(ctx, `select id,application_id,workflow_name,calendar_name,local_time,missed_policy,input,next_at from start_schedules where next_at<=now() order by next_at limit 100`)
	if e != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, app, wf, cal, clock, policy string
		var data []byte
		var due time.Time
		if rows.Scan(&id, &app, &wf, &cal, &clock, &policy, &data, &due) != nil {
			continue
		}
		d, e := s.loadCalendar(ctx, app, cal)
		if e != nil {
			continue
		}
		from := time.Now().UTC()
		if policy == "catch_up" {
			from = due.Add(time.Second)
		}
		next, e := nextCalendar(d, from, clock)
		if e != nil {
			continue
		}
		tag, _ := s.db.Exec(ctx, `update start_schedules set next_at=$2,updated_at=now() where id=$1 and next_at=$3`, id, next, due)
		if tag.RowsAffected() != 1 || policy == "skip" {
			continue
		}
		s.startWorkflow(ctx, app, frame{Name: wf, Data: data, IdempotencyKey: "start:" + id + ":" + due.Format(time.RFC3339Nano)})
	}
}
func (s *Server) dashboardStats(ctx context.Context) reply {
	var scheduled, pending, blocked, workflows int
	e := s.db.QueryRow(ctx, `select (select count(*) from schedules where status='scheduled'),(select count(*) from deliveries where status='pending'),(select count(*) from deliveries where status='blocked'),(select count(*) from workflow_instances where status!='completed')`).Scan(&scheduled, &pending, &blocked, &workflows)
	if e != nil {
		return fail(e.Error())
	}
	return ok(map[string]int{"scheduled": scheduled, "pending": pending, "blocked": blocked, "workflows": workflows})
}
func (s *Server) replayDelivery(ctx context.Context, f frame) reply {
	if f.DeliveryID == "" {
		return fail("deliveryId required")
	}
	tag, e := s.db.Exec(ctx, `update deliveries set status='pending',attempts=0,next_attempt_at=now(),locked_until=null,lease_owner=null where id=$1 and status in ('blocked','cancelled')`, f.DeliveryID)
	if e != nil {
		return fail(e.Error())
	}
	return ok(map[string]bool{"replayed": tag.RowsAffected() == 1})
}
func (s *Server) cancelDelivery(ctx context.Context, f frame) reply {
	if f.DeliveryID == "" {
		return fail("deliveryId required")
	}
	tag, e := s.db.Exec(ctx, `update deliveries set status='cancelled' where id=$1 and status in ('pending','blocked')`, f.DeliveryID)
	if e != nil {
		return fail(e.Error())
	}
	return ok(map[string]bool{"cancelled": tag.RowsAffected() == 1})
}

var _ = fmt.Sprintf
