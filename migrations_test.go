package calcron_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bzrlab/calcron"
	"github.com/jackc/pgx/v5/pgxpool"
)

var schemaSeq atomic.Int64

// schemaPool returns a pool whose search_path is a fresh schema. The schema is dropped on cleanup.
func schemaPool(t *testing.T, admin *pgxpool.Pool, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	// UnixNano alone can collide across goroutines; the counter makes the name unique.
	schema := fmt.Sprintf("migration_%d_%d", time.Now().UnixNano(), schemaSeq.Add(1))
	if _, err := admin.Exec(ctx, "create schema "+schema); err != nil {
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
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func adminPool(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	databaseURL := os.Getenv("CALCRON_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CALCRON_TEST_DATABASE_URL required")
	}
	admin, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	return admin, databaseURL
}

// The migration must only look at its own schema: a constraint of the same name in a
// sibling schema must not make it skip creating the constraint here.
func TestMigrationConstraintsAreScopedToItsOwnSchema(t *testing.T) {
	ctx := context.Background()
	admin, databaseURL := adminPool(t)
	first := schemaPool(t, admin, databaseURL)
	second := schemaPool(t, admin, databaseURL)
	for _, pool := range []*pgxpool.Pool{first, second} {
		if _, err := pool.Exec(ctx, calcron.InitialMigration); err != nil {
			t.Fatal(err)
		}
		var n int
		err := pool.QueryRow(ctx, `select count(*) from pg_constraint where connamespace = current_schema()::regnamespace
 and conname in ('deliveries_workflow_instance_id_fkey','start_schedules_status_check','workflow_instances_status_check')`).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		if n != 3 {
			t.Fatalf("constraints in own schema: %d, want 3", n)
		}
	}
}

// Migrating while sibling schemas are created and dropped must never fail: catalog
// probes that touch other schemas hit relations dropped mid-scan (SQLSTATE XX000).
func TestMigrationSurvivesSiblingSchemasBeingDropped(t *testing.T) {
	ctx := context.Background()
	admin, databaseURL := adminPool(t)
	stop := make(chan struct{})
	var churn sync.WaitGroup
	for i := 0; i < 4; i++ {
		churn.Add(1)
		go func() {
			defer churn.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				name := fmt.Sprintf("migration_churn_%d_%d", time.Now().UnixNano(), schemaSeq.Add(1))
				_, _ = admin.Exec(ctx, "create schema "+name)
				conn, err := admin.Acquire(ctx)
				if err == nil {
					_, _ = conn.Exec(ctx, "set search_path to "+name)
					_, _ = conn.Exec(ctx, calcron.InitialMigration)
					_, _ = conn.Exec(ctx, "reset search_path")
					conn.Release()
				}
				_, _ = admin.Exec(ctx, "drop schema "+name+" cascade")
			}
		}()
	}
	t.Cleanup(func() { close(stop); churn.Wait() })
	for i := 0; i < 20; i++ {
		pool := schemaPool(t, admin, databaseURL)
		if _, err := pool.Exec(ctx, calcron.InitialMigration); err != nil {
			t.Fatalf("migration %d: %v", i, err)
		}
	}
}
