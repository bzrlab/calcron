package idempotency_test

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/bzrlab/calcron"
	"github.com/bzrlab/calcron/internal/idempotency"
	"github.com/bzrlab/calcron/internal/protocol"
	"github.com/jackc/pgx/v5/pgxpool"
)

// smallPool returns an isolated-schema pool capped at maxConns connections.
func smallPool(t *testing.T, maxConns int32) (context.Context, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("CALCRON_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CALCRON_TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "idem_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err = admin.Exec(ctx, "create schema "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "drop schema "+schema+" cascade") })
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	config, err := pgxpool.ParseConfig(parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = maxConns
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if _, err = db.Exec(ctx, calcron.InitialMigration); err != nil {
		t.Fatal(err)
	}
	return ctx, db
}

// A command run inside Serial needs its own connection. Callers outnumbering the
// pool must queue instead of every pool connection becoming a lock holder or
// lock waiter with none left for the command to run on.
func TestSerialDoesNotExhaustPoolWhenCallersOutnumberConnections(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  func(int) string
	}{
		{"same key", func(int) string { return "shared" }},
		{"distinct keys", func(i int) string { return "key-" + strconv.Itoa(i) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, db := smallPool(t, 4)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel() // unblocks stuck callers so a failure does not hang cleanup
			done := make(chan struct{})
			go func() {
				defer close(done)
				var wg sync.WaitGroup
				for i := 0; i < 32; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						idempotency.Serial(ctx, db, "app", tc.key(i), func() protocol.Reply {
							var one int
							if err := db.QueryRow(ctx, `select 1`).Scan(&one); err != nil {
								return protocol.Reply{Error: err.Error()}
							}
							return protocol.Reply{OK: true}
						})
					}()
				}
				wg.Wait()
			}()
			select {
			case <-done:
			case <-time.After(20 * time.Second):
				t.Fatal("Serial deadlocked: every pool connection held while commands waited for another")
			}
		})
	}
}
