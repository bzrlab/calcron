package cron

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestClientReconnects(t *testing.T) {
	var connections atomic.Int32
	reconnected := make(chan struct{}, 1)
	h := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		_, _, err = c.Read(r.Context())
		if err != nil {
			return
		}
		b, _ := json.Marshal(map[string]any{"ok": true})
		_ = c.Write(r.Context(), websocket.MessageText, b)
		if connections.Add(1) == 1 {
			time.Sleep(50 * time.Millisecond)
			_ = c.Close(websocket.StatusGoingAway, "test")
			return
		}
		reconnected <- struct{}{}
		<-r.Context().Done()
	})}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("loopback unavailable")
	}
	go h.Serve(l)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, err := Connect(ctx, "ws://"+l.Addr().String(), "token")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	select {
	case <-reconnected:
	case <-ctx.Done():
		t.Fatal("client did not reconnect")
	}
}
