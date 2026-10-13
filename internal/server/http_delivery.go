package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// deliverCallbacks pushes pending Deliveries for Applications that have a callback
// and no live WebSocket. Connected Applications stay on deliver.
func (s *Server) deliverCallbacks(ctx context.Context) {
	rows, err := s.db.Query(ctx, `
		select d.id
		from deliveries d
		join applications a on a.id = d.application_id
		where d.status = 'pending'
		  and d.next_attempt_at <= now()
		  and (d.locked_until is null or d.locked_until <= now())
		  and a.callback_url is not null
		  and not (d.application_id = any($1::text[]))
		order by d.next_attempt_at
		limit 100`, s.hub.connected())
	if err != nil {
		return
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	if rows.Err() != nil {
		return
	}
	rows.Close()
	for _, id := range ids {
		s.pushCallback(ctx, id)
	}
}

func (s *Server) pushCallback(ctx context.Context, id string) {
	var app, scheduleID, event, callbackURL, secret string
	var payload []byte
	err := s.db.QueryRow(ctx, `
		with candidate as (
			select id from deliveries
			where id=$1 and status='pending' and next_attempt_at<=now()
			  and (locked_until is null or locked_until<=now())
			for update skip locked
		)
		update deliveries d
		set locked_until=now()+interval '15 seconds', lease_owner=$2
		from candidate where d.id=candidate.id
		returning d.id, d.application_id, coalesce(d.schedule_id,''), d.event, d.payload,
			coalesce((select callback_url from applications where id=d.application_id), ''),
			coalesce((select callback_secret from applications where id=d.application_id), '')`,
		id, s.id).Scan(&id, &app, &scheduleID, &event, &payload, &callbackURL, &secret)
	if err != nil || callbackURL == "" || secret == "" {
		return
	}
	s.callbackWait.Add(1)
	go s.postCallback(ctx, id, app, scheduleID, event, payload, callbackURL, secret)
}

func (s *Server) postCallback(ctx context.Context, id, app, scheduleID, event string, payload []byte, callbackURL, secret string) {
	defer s.callbackWait.Done()
	if !s.sendCallback(ctx, id, scheduleID, event, payload, callbackURL, secret) {
		s.releaseDelivery(ctx, id)
		return
	}
	if !s.ack(ctx, app, frame{DeliveryID: id}).OK {
		s.releaseDelivery(ctx, id)
	}
}

func (s *Server) sendCallback(ctx context.Context, id, scheduleID, event string, payload []byte, callbackURL, secret string) bool {
	body, err := json.Marshal(map[string]any{
		"op": "delivery", "deliveryId": id, "scheduleId": scheduleID, "event": event, "data": json.RawMessage(payload),
	})
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Calcron-Callback-Secret", secret)
	res, err := s.callbackHTTP.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	// ponytail: any non-2xx releases the lease and does not count a failed acknowledgement. Split refused from unreachable in the offline-callback ticket.
	return res.StatusCode >= 200 && res.StatusCode < 300
}

func (s *Server) releaseDelivery(ctx context.Context, id string) {
	ctx = context.WithoutCancel(ctx)
	_, _ = s.db.Exec(ctx, `update deliveries set locked_until=null,lease_owner=null where id=$1 and lease_owner=$2 and status='pending'`, id, s.id)
}
