package server

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

func idempotent(ctx context.Context, row pgx.Row) (reply, bool) {
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return reply{}, false
	}
	var stored reply
	return stored, json.Unmarshal(raw, &stored) == nil
}

// serial returns the original response for a reused idempotency key and runs
// a new command while holding a per-application advisory lock.
func (s *Server) serial(ctx context.Context, app, key string, run func() reply) reply {
	connection, err := s.db.Acquire(ctx)
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

func rememberTx(ctx context.Context, tx pgx.Tx, app, key string, response reply) error {
	body, _ := json.Marshal(response)
	_, err := tx.Exec(ctx, `insert into idempotency(application_id,key,response) values($1,$2,$3)`, app, key, body)
	return err
}
