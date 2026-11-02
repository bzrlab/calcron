// Package retention removes expired finished work in bounded batches.
package retention

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const BatchSize = 100
const DefaultPeriod = 30 * 24 * time.Hour

type Counts struct{ Deliveries, Schedules, History, Idempotency int64 }

// Cleanup deletes at most limit rows per table, oldest first. Live subjects keep
// their history; workflow and start-schedule history remain retained indefinitely.
func Cleanup(ctx context.Context, db *pgxpool.Pool, cutoff time.Time, limit int) (Counts, error) {
	var counts Counts
	if limit <= 0 {
		return counts, fmt.Errorf("retention batch size must be positive")
	}
	queries := []struct {
		sql   string
		count *int64
	}{
		{`with expired as (
 select d.id from deliveries d where d.status in ('acked','cancelled') and d.finished_at < $1
 and (d.locked_until is null or d.locked_until <= now())
 and not exists (select 1 from workflow_instances w where w.id=d.workflow_instance_id and w.status not in ('completed','cancelled','failed'))
 order by d.finished_at,d.id limit $2 for update of d skip locked
 ) delete from deliveries d using expired e where d.id=e.id`, &counts.Deliveries},
		{`with expired as (
 select s.id from schedules s where s.status in ('delivered','cancelled') and s.updated_at < $1
 and not s.awaiting_signal
 and not exists (select 1 from deliveries d where d.schedule_id=s.id)
 order by s.updated_at,s.id limit $2 for update of s skip locked
 ) delete from schedules s using expired e where s.id=e.id`, &counts.Schedules},
		{`with expired as (
 select h.id from history h where h.created_at < $1 and (
 (h.subject_type='schedule' and not exists (select 1 from schedules s where s.id=h.subject_id)) or
 (h.subject_type='delivery' and not exists (select 1 from deliveries d where d.id=h.subject_id)))
 order by h.created_at,h.id limit $2 for update of h skip locked
 ) delete from history h using expired e where h.id=e.id`, &counts.History},
		{`with expired as (
 select application_id,key from idempotency where created_at < $1
 order by created_at,application_id,key limit $2 for update skip locked
 ) delete from idempotency i using expired e where i.application_id=e.application_id and i.key=e.key`, &counts.Idempotency},
	}
	var failures []error
	for _, query := range queries {
		// Pool pressure gets its own bounded wait, independent of query execution.
		acquireCtx, cancelAcquire := context.WithTimeout(ctx, time.Second)
		conn, err := db.Acquire(acquireCtx)
		cancelAcquire()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		affected, err := cleanupTable(ctx, conn, query.sql, cutoff, limit)
		conn.Release()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		*query.count = affected
	}
	return counts, errors.Join(failures...)
}

func cleanupTable(ctx context.Context, conn *pgxpool.Conn, sql string, cutoff time.Time, limit int) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `set local lock_timeout = '50ms'; set local statement_timeout = '200ms'`); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, sql, cutoff, limit)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
