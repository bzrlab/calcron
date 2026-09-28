package server

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bzrlab/calcron/internal/workflow"
	"github.com/google/cel-go/cel"
)

type workflowDefinition = workflow.Definition
type workflowState = workflow.State

func parseWorkflow(raw json.RawMessage) (workflowDefinition, error) {
	return workflow.Parse(raw)
}
func compileCEL(expr string) (cel.Program, error) {
	return workflow.CompileCEL(expr)
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
