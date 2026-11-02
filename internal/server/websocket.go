package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

type hub struct {
	sync.RWMutex
	apps map[string]map[*peer]struct{}
}

type peer struct {
	conn        *websocket.Conn
	mu          sync.Mutex
	app         string
	admin       bool
	maxInFlight int
	inFlight    map[string]struct{}
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(s.maxPayloadBytes + (64 << 10) + 1)
	p := &peer{conn: c}
	ctx := r.Context()
	body, err := s.readCommand(ctx, c)
	if err != nil {
		return
	}
	var auth frame
	if json.Unmarshal(body, &auth) != nil || auth.Op != "auth" {
		return
	}
	if auth.MaxInFlight < 0 {
		_ = p.send(ctx, reply{ID: auth.ID, Error: "maxInFlight must be a non-negative integer"})
		return
	}
	p.maxInFlight = auth.MaxInFlight
	p.inFlight = map[string]struct{}{}
	app, admin, err := s.commands.authenticate(ctx, auth.Token)
	if err != nil {
		code := "unavailable"
		if errors.Is(err, errBadToken) {
			code = "unauthorized"
		}
		_ = p.send(ctx, reply{ID: auth.ID, Error: code})
		return
	}
	p.app, p.admin = app, admin
	if !admin {
		s.hub.add(p)
	}
	defer s.hub.remove(p)
	_ = p.send(ctx, reply{ID: auth.ID, OK: true, Data: map[string]any{"application": app, "admin": admin}})
	for {
		body, err = s.readCommand(ctx, c)
		if err != nil {
			return
		}
		var command frame
		if err := json.Unmarshal(body, &command); err != nil {
			_ = p.send(ctx, reply{Error: "bad json"})
			continue
		}
		res := s.commands.handle(ctx, p, command)
		res.ID = command.ID
		_ = p.send(ctx, res)
	}
}

func (p *peer) send(ctx context.Context, value any) error {
	body, _ := json.Marshal(value)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.conn.Write(ctx, websocket.MessageText, body)
}

func (h *hub) add(p *peer) {
	h.Lock()
	defer h.Unlock()
	if h.apps[p.app] == nil {
		h.apps[p.app] = map[*peer]struct{}{}
	}
	h.apps[p.app][p] = struct{}{}
}

func (h *hub) remove(p *peer) {
	h.Lock()
	defer h.Unlock()
	delete(h.apps[p.app], p)
	if len(h.apps[p.app]) == 0 {
		delete(h.apps, p.app)
	}
}

func (h *hub) peers(app string) []*peer {
	h.RLock()
	defer h.RUnlock()
	peers := make([]*peer, 0, len(h.apps[app]))
	for p := range h.apps[app] {
		peers = append(peers, p)
	}
	return peers
}

func (h *hub) connected() []string {
	h.RLock()
	defer h.RUnlock()
	apps := make([]string, 0, len(h.apps))
	for app := range h.apps {
		apps = append(apps, app)
	}
	return apps
}

func (h *hub) peer(app string) *peer {
	h.RLock()
	defer h.RUnlock()
	for p := range h.apps[app] {
		return p
	}
	return nil
}

// Read one sentinel byte beyond the command budget so both transports report
// the same error without allocating an unbounded WebSocket message.
func (s *Server) readCommand(ctx context.Context, c *websocket.Conn) ([]byte, error) {
	_, reader, err := c.Reader(ctx)
	if err != nil {
		return nil, err
	}
	limit := s.maxPayloadBytes + (64 << 10)
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if int64(len(body)) > limit {
		_ = c.Close(websocket.StatusMessageTooBig, "payload too large")
		return nil, errors.New("payload too large")
	}
	return body, err
}
