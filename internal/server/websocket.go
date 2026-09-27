package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

type hub struct {
	sync.RWMutex
	apps map[string]map[*peer]struct{}
}

type peer struct {
	conn  *websocket.Conn
	mu    sync.Mutex
	app   string
	admin bool
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	p := &peer{conn: c}
	ctx := r.Context()
	_, body, err := c.Read(ctx)
	if err != nil {
		return
	}
	var auth frame
	if json.Unmarshal(body, &auth) != nil || auth.Op != "auth" {
		return
	}
	app, admin, err := s.commands.authenticate(ctx, auth.Token)
	if err != nil {
		_ = p.send(ctx, reply{ID: auth.ID, Error: "unauthorized"})
		return
	}
	p.app, p.admin = app, admin
	if !admin {
		s.hub.add(p)
	}
	defer s.hub.remove(p)
	_ = p.send(ctx, reply{ID: auth.ID, OK: true, Data: map[string]any{"application": app, "admin": admin}})
	for {
		_, body, err = c.Read(ctx)
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
