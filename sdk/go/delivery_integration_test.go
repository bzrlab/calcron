package cron_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bzrlab/calcron"
	"github.com/bzrlab/calcron/internal/server"
	cron "github.com/bzrlab/calcron/sdk/go"
	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEnqueueDeliversPayloadBoundary(t *testing.T) {
	databaseURL := os.Getenv("CALCRON_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CALCRON_TEST_DATABASE_URL required")
	}
	// Isolate migrations and workers from other packages using the same test database.
	db, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := fmt.Sprintf("sdk_receive_%d", time.Now().UnixNano())
	if _, err = db.Exec(context.Background(), "create schema "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(context.Background(), "drop schema "+schema+" cascade"); err != nil {
			t.Error(err)
		}
	}()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	databaseURL = parsed.String()
	for _, size := range []int{40 << 10, 64 << 10, 256 << 10} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			config := server.Config{DatabaseURL: databaseURL, AdminToken: "receive-test", Migration: calcron.InitialMigration}
			var options cron.ConnectOptions
			if size > 64<<10 {
				config.MaxPayloadBytes = int64(size)
				options.MaxPayloadBytes = int64(size)
			}
			s, err := server.New(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			h := httptest.NewServer(s.Handler())
			defer h.Close()
			url := "ws" + strings.TrimPrefix(h.URL, "http") + "/ws"
			admin, _, err := websocket.Dial(ctx, url, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer admin.CloseNow()
			command := func(v any) map[string]any {
				t.Helper()
				raw, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				if err = admin.Write(ctx, websocket.MessageText, raw); err != nil {
					t.Fatal(err)
				}
				_, raw, err = admin.Read(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var reply map[string]any
				if err = json.Unmarshal(raw, &reply); err != nil || reply["ok"] != true {
					t.Fatalf("reply: %s (%v)", raw, err)
				}
				return reply
			}
			command(map[string]any{"op": "auth", "token": "receive-test"})
			created := command(map[string]any{"op": "app.create", "name": "receive-boundary", "namespace": fmt.Sprintf("receive-%d", time.Now().UnixNano())})
			token := created["data"].(map[string]any)["token"].(string)
			client, err := cron.Connect(ctx, url, token, options)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			events := make(chan cron.Event, 1)
			client.On("large", func(e cron.Event) { events <- e })
			payload := json.RawMessage(`"` + strings.Repeat("x", size-2) + `"`)
			if _, err = client.Enqueue(ctx, cron.Enqueue{Event: "large", Data: payload}); err != nil {
				t.Fatal(err)
			}
			select {
			case e := <-events:
				if string(e.Data) != string(payload) {
					t.Fatal("delivery payload changed")
				}
				if ack, err := e.Ack(ctx); err != nil || !ack.Acked {
					t.Fatalf("ack: %#v, %v", ack, err)
				}
			case <-ctx.Done():
				t.Fatal("enqueued payload did not reach handler:", ctx.Err())
			}
		})
	}
}
