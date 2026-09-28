package server

import (
	"context"

	"github.com/bzrlab/calcron/internal/idempotency"
	"github.com/jackc/pgx/v5"
)

func (s *Server) serial(ctx context.Context, app, key string, run func() reply) reply {
	return idempotency.Serial(ctx, s.db, app, key, run)
}

func rememberTx(ctx context.Context, tx pgx.Tx, app, key string, response reply) error {
	return idempotency.RememberTx(ctx, tx, app, key, response)
}
