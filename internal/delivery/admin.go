// Package delivery owns durable delivery administration.
package delivery

import (
	"context"
	"github.com/bzrlab/calcron/internal/protocol"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Admin struct{ db *pgxpool.Pool }

func NewAdmin(db *pgxpool.Pool) *Admin { return &Admin{db: db} }
func (a *Admin) Stats(ctx context.Context) protocol.Reply {
	var scheduled, pending, blocked, workflows int
	err := a.db.QueryRow(ctx, `select (select count(*) from schedules where status='scheduled'),(select count(*) from deliveries where status='pending'),(select count(*) from deliveries where status='blocked'),(select count(*) from workflow_instances where status!='completed')`).Scan(&scheduled, &pending, &blocked, &workflows)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]int{"scheduled": scheduled, "pending": pending, "blocked": blocked, "workflows": workflows})
}
func (a *Admin) Replay(ctx context.Context, f protocol.Frame) protocol.Reply {
	if f.DeliveryID == "" {
		return fail("deliveryId required")
	}
	tag, err := a.db.Exec(ctx, `update deliveries set status='pending',attempts=0,next_attempt_at=now(),locked_until=null,lease_owner=null where id=$1 and status in ('blocked','cancelled')`, f.DeliveryID)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]bool{"replayed": tag.RowsAffected() == 1})
}
func (a *Admin) Cancel(ctx context.Context, f protocol.Frame) protocol.Reply {
	if f.DeliveryID == "" {
		return fail("deliveryId required")
	}
	tag, err := a.db.Exec(ctx, `update deliveries set status='cancelled' where id=$1 and status in ('pending','blocked')`, f.DeliveryID)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]bool{"cancelled": tag.RowsAffected() == 1})
}
func fail(err string) protocol.Reply { return protocol.Reply{Error: err} }
func ok(data any) protocol.Reply     { return protocol.Reply{OK: true, Data: data} }
