package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
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
	var attempts int
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
			coalesce((select callback_secret from applications where id=d.application_id), ''),
			d.attempts`,
		id, s.id).Scan(&id, &app, &scheduleID, &event, &payload, &callbackURL, &secret, &attempts)
	if err != nil || callbackURL == "" || secret == "" {
		return
	}
	if attempts >= 5 {
		_, _ = s.db.Exec(ctx, `update deliveries set status='blocked',locked_until=null,lease_owner=null where id=$1 and lease_owner=$2`, id, s.id)
		return
	}
	s.callbackWait.Add(1)
	go s.postCallback(ctx, id, app, scheduleID, event, payload, callbackURL, secret, attempts)
}

func (s *Server) postCallback(ctx context.Context, id, app, scheduleID, event string, payload []byte, callbackURL, secret string, attempts int) {
	defer s.callbackWait.Done()
	status, err := s.sendCallback(ctx, id, scheduleID, event, payload, callbackURL, secret)
	if err != nil {
		// Unreachable callback is an offline Application: do not count a failed acknowledgement.
		s.releaseDelivery(ctx, id)
		return
	}
	if status < 200 || status >= 300 {
		s.countRefusal(ctx, id, attempts)
		return
	}
	if !s.ack(ctx, app, frame{DeliveryID: id}).OK {
		s.releaseDelivery(ctx, id)
	}
}

func (s *Server) sendCallback(ctx context.Context, id, scheduleID, event string, payload []byte, callbackURL, secret string) (int, error) {
	body, err := json.Marshal(map[string]any{
		"op": "delivery", "deliveryId": id, "scheduleId": scheduleID, "event": event, "data": json.RawMessage(payload),
	})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Calcron-Callback-Secret", secret)
	res, err := s.callbackHTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	return res.StatusCode, nil
}

func (s *Server) countRefusal(ctx context.Context, id string, attempts int) {
	ctx = context.WithoutCancel(ctx)
	base := s.retryBase
	if base == 0 {
		base = time.Second
	}
	delay := base * time.Duration(1<<attempts)
	_, _ = s.db.Exec(ctx, `update deliveries set attempts=attempts+1,last_sent_at=now(),next_attempt_at=now()+$2::interval,locked_until=null,lease_owner=null where id=$1 and lease_owner=$3 and status='pending'`, id, delay.String(), s.id)
}

func (s *Server) releaseDelivery(ctx context.Context, id string) {
	ctx = context.WithoutCancel(ctx)
	_, _ = s.db.Exec(ctx, `update deliveries set locked_until=null,lease_owner=null where id=$1 and lease_owner=$2 and status='pending'`, id, s.id)
}
