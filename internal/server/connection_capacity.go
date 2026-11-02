package server

import "context"

// All accounting is protected by the hub lock. A retry retains its original
// connection's slot even after its database lease expires.
func (h *hub) reserve(app, id string) (*peer, bool) {
	h.Lock()
	defer h.Unlock()
	for p := range h.apps[app] {
		if _, ok := p.inFlight[id]; ok {
			return p, false
		}
	}
	for p := range h.apps[app] {
		if p.maxInFlight == 0 || len(p.inFlight) < p.maxInFlight {
			p.inFlight[id] = struct{}{}
			return p, true
		}
	}
	return nil, false
}

func (h *hub) release(p *peer, id string) {
	h.Lock()
	defer h.Unlock()
	delete(p.inFlight, id)
}

func (h *hub) deliveryCandidates() (apps, available, held []string) {
	h.RLock()
	defer h.RUnlock()
	apps, available, held = []string{}, []string{}, []string{}
	for app, peers := range h.apps {
		apps = append(apps, app)
		hasCapacity := false
		for p := range peers {
			if p.maxInFlight == 0 || len(p.inFlight) < p.maxInFlight {
				hasCapacity = true
			}
			for id := range p.inFlight {
				held = append(held, id)
			}
		}
		if hasCapacity {
			available = append(available, app)
		}
	}
	return
}

// Consult committed state rather than ack responses: HTTP acks and cached
// idempotent responses must have exactly the same accounting as WebSocket acks.
func (s *Server) reconcileInFlight(ctx context.Context) bool {
	_, _, held := s.hub.deliveryCandidates()
	if len(held) == 0 {
		return true
	}
	rows, err := s.db.Query(ctx, `select id from deliveries where id=any($1) and status in ('pending','blocked')`, held)
	if err != nil {
		return false
	}
	defer rows.Close()
	active := map[string]bool{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			return false
		}
		active[id] = true
	}
	if rows.Err() != nil {
		return false
	}
	s.hub.Lock()
	defer s.hub.Unlock()
	for _, peers := range s.hub.apps {
		for p := range peers {
			for id := range p.inFlight {
				if !active[id] {
					delete(p.inFlight, id)
				}
			}
		}
	}
	return true
}
