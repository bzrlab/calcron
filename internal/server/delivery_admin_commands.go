package server

import "context"

func (s *Server) dashboardStats(ctx context.Context) reply {
	var scheduled, pending, blocked, workflows int
	err := s.db.QueryRow(ctx, `select (select count(*) from schedules where status='scheduled'),(select count(*) from deliveries where status='pending'),(select count(*) from deliveries where status='blocked'),(select count(*) from workflow_instances where status!='completed')`).Scan(&scheduled, &pending, &blocked, &workflows)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]int{"scheduled": scheduled, "pending": pending, "blocked": blocked, "workflows": workflows})
}

func (s *Server) replayDelivery(ctx context.Context, f frame) reply {
	if f.DeliveryID == "" {
		return fail("deliveryId required")
	}
	tag, err := s.db.Exec(ctx, `update deliveries set status='pending',attempts=0,next_attempt_at=now(),locked_until=null,lease_owner=null where id=$1 and status in ('blocked','cancelled')`, f.DeliveryID)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]bool{"replayed": tag.RowsAffected() == 1})
}

func (s *Server) cancelDelivery(ctx context.Context, f frame) reply {
	if f.DeliveryID == "" {
		return fail("deliveryId required")
	}
	tag, err := s.db.Exec(ctx, `update deliveries set status='cancelled' where id=$1 and status in ('pending','blocked')`, f.DeliveryID)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]bool{"cancelled": tag.RowsAffected() == 1})
}
