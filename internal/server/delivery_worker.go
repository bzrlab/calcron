package server

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/calcron/calcron/internal/schedule"
	"github.com/jackc/pgx/v5"
)

func (s *Server) ack(ctx context.Context, app string, f frame) reply {
	if f.DeliveryID == "" {
		return fail("deliveryId required")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	var scheduleID, workflowID, nextState *string
	err = tx.QueryRow(ctx, `update deliveries set status='acked',acked_at=now() where id=$1 and application_id=$2 and status='pending' returning schedule_id,workflow_instance_id,next_state`, f.DeliveryID, app).Scan(&scheduleID, &workflowID, &nextState)
	if errors.Is(err, pgx.ErrNoRows) {
		return ok(map[string]bool{"acked": false})
	}
	if err != nil {
		return fail(err.Error())
	}
	if scheduleID != nil {
		var raw []byte
		if err = tx.QueryRow(ctx, `select coalesce(chain,'null'::jsonb) from schedules where id=$1`, *scheduleID).Scan(&raw); err != nil {
			return fail(err.Error())
		}
		if next, parseErr := schedule.ParseChain(raw); parseErr != nil {
			return fail(parseErr.Error())
		} else if next.Key != "" {
			if response := s.schedules.SetImmediateTx(ctx, tx, app, frame{Key: next.Key, Event: next.Event, After: next.After, Data: next.Data}); !response.OK {
				return fail("chain failed")
			}
		}
	}
	if workflowID != nil && nextState != nil {
		if _, err = tx.Exec(ctx, `update workflow_instances set current_state=$2,status='running',updated_at=now() where id=$1 and status='waiting_ack'`, *workflowID, *nextState); err != nil {
			return fail(err.Error())
		}
	}
	if _, err = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event) values($1,'delivery',$2,'acked')`, app, f.DeliveryID); err != nil {
		return fail(err.Error())
	}
	response := ok(map[string]bool{"acked": true})
	if err = rememberTx(ctx, tx, app, f.IdempotencyKey, response); err != nil {
		return fail(err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err.Error())
	}
	if workflowID != nil && nextState != nil {
		s.workflows.run(ctx, *workflowID)
	}
	return response
}

func (s *Server) makeDue(ctx context.Context) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `select id,application_id,event,payload from schedules where status='scheduled' and run_at<=now() order by run_at limit 100 for update skip locked`)
	if err != nil {
		return
	}
	type dueSchedule struct {
		id, app, event string
		data           []byte
	}
	var due []dueSchedule
	for rows.Next() {
		var item dueSchedule
		if rows.Scan(&item.id, &item.app, &item.event, &item.data) == nil {
			due = append(due, item)
		}
	}
	rows.Close()
	for _, item := range due {
		if _, err = tx.Exec(ctx, `update schedules set status='delivered',updated_at=now() where id=$1`, item.id); err == nil {
			_, err = tx.Exec(ctx, `insert into deliveries(id,schedule_id,application_id,event,payload,status,next_attempt_at) values($2,$1,$3,$4,$5,'pending',now())`, item.id, random(), item.app, item.event, item.data)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event) values($2,'schedule',$1,'due')`, item.id, item.app)
		}
		if err != nil {
			return
		}
	}
	_ = tx.Commit(ctx)
}

func (s *Server) deliver(ctx context.Context) {
	rows, err := s.db.Query(ctx, `select id,application_id,coalesce(schedule_id,''),event,payload,attempts from deliveries where status='pending' and next_attempt_at<=now() order by next_attempt_at limit 100`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, app, scheduleID, event string
		var payload []byte
		var attempts int
		if rows.Scan(&id, &app, &scheduleID, &event, &payload, &attempts) != nil {
			continue
		}
		peer := s.hub.peer(app)
		if peer == nil {
			continue
		}
		var claimedID, claimedApp, claimedSchedule, claimedEvent string
		var claimedData []byte
		var claimedAttempts int
		err = s.db.QueryRow(ctx, `with candidate as (select id from deliveries where id=$1 and status='pending' and next_attempt_at<=now() and (locked_until is null or locked_until<=now()) for update skip locked) update deliveries d set locked_until=now()+interval '15 seconds',lease_owner=$2 from candidate where d.id=candidate.id returning d.id,d.application_id,coalesce(d.schedule_id,''),d.event,d.payload,d.attempts`, id, s.id).Scan(&claimedID, &claimedApp, &claimedSchedule, &claimedEvent, &claimedData, &claimedAttempts)
		if err != nil {
			continue
		}
		if claimedAttempts >= 5 {
			_, _ = s.db.Exec(ctx, `update deliveries set status='blocked',locked_until=null,lease_owner=null where id=$1 and lease_owner=$2`, claimedID, s.id)
			continue
		}
		if peer.send(ctx, map[string]any{"op": "delivery", "deliveryId": claimedID, "scheduleId": claimedSchedule, "event": claimedEvent, "data": json.RawMessage(claimedData)}) != nil {
			_, _ = s.db.Exec(ctx, `update deliveries set locked_until=null,lease_owner=null where id=$1 and lease_owner=$2 and status='pending'`, claimedID, s.id)
			continue
		}
		base := s.retryBase
		if base == 0 {
			base = time.Second
		}
		delay := base * time.Duration(1<<claimedAttempts)
		_, _ = s.db.Exec(ctx, `update deliveries set attempts=attempts+1,last_sent_at=now(),next_attempt_at=now()+$2::interval,locked_until=null,lease_owner=null where id=$1 and lease_owner=$3 and status='pending'`, claimedID, delay.String(), s.id)
	}
}
