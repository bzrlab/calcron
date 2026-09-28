package idempotency

import (
	"context"
	"encoding/json"

	"github.com/calcron/calcron/internal/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func idempotent(ctx context.Context, row pgx.Row) (protocol.Reply, bool) {
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return protocol.Reply{}, false
	}
	var stored protocol.Reply
	return stored, json.Unmarshal(raw, &stored) == nil
}

// Serial returns the original response for a reused idempotency key and runs
// a new command while holding a per-application advisory lock.
func Serial(ctx context.Context, db *pgxpool.Pool, app, key string, run func() protocol.Reply) protocol.Reply {
	connection, err := db.Acquire(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer connection.Release()
	lockKey := app + ":" + key
	if _, err = connection.Exec(ctx, `select pg_advisory_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return fail(err.Error())
	}
	defer connection.Exec(context.Background(), `select pg_advisory_unlock(hashtextextended($1,0))`, lockKey)
	if response, found := idempotent(ctx, connection.QueryRow(ctx, `select response from idempotency where application_id=$1 and key=$2`, app, key)); found {
		return response
	}
	return run()
}

func RememberTx(ctx context.Context, tx pgx.Tx, app, key string, response protocol.Reply) error {
	body, _ := json.Marshal(response)
	_, err := tx.Exec(ctx, `insert into idempotency(application_id,key,response) values($1,$2,$3)`, app, key, body)
	return err
}

func fail(err string) protocol.Reply { return protocol.Reply{Error: err} }
