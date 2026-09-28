package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type commandRouter struct{ server *Server }

func newCommandRouter(server *Server) *commandRouter { return &commandRouter{server: server} }

func (r *commandRouter) authenticate(ctx context.Context, token string) (string, bool, error) {
	s := r.server
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.admin)) == 1 {
		return "admin", true, nil
	}
	parts := strings.Split(token, "_")
	if len(parts) != 3 || parts[0] != "cc" {
		return "", false, errors.New("bad token")
	}
	var app, hash string
	err := s.db.QueryRow(ctx, `select t.application_id,t.secret_hash from application_tokens t where t.id=$1 and t.revoked_at is null`, parts[1]).Scan(&app, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(parts[2])) != nil {
		return "", false, errors.New("bad token")
	}
	return app, false, nil
}

func (r *commandRouter) handle(ctx context.Context, p *peer, f frame) reply {
	s := r.server
	if p.admin {
		if err := validateAdminOperation(f.Op); err != "" {
			return fail(err)
		}
		switch commandOperation(f.Op) {
		case operationAppCreate:
			return s.applications.Create(ctx, f)
		case operationAppTokenRotate:
			return s.applications.RotateToken(ctx, f)
		case operationAppTokenRevoke:
			return s.applications.RevokeToken(ctx, f)
		case operationWorkflowPublish:
			return s.publishWorkflow(ctx, f)
		case operationCalendarSet:
			return s.calendars.Set(ctx, f)
		case operationCalendarNext:
			return s.calendars.Next(ctx, f)
		case operationCalendarOccurrences:
			return s.calendars.Occurrences(ctx, f)
		case operationStartScheduleSet:
			return s.schedules.SetStart(ctx, f)
		case operationDashboardStats:
			return s.dashboardStats(ctx)
		case operationDashboardList:
			return s.dashboardReads.list(ctx, f)
		case operationDeliveryReplay:
			return s.replayDelivery(ctx, f)
		case operationDeliveryCancel:
			return s.cancelDelivery(ctx, f)
		}
	}
	if f.IdempotencyKey == "" {
		return fail("idempotencyKey required")
	}
	return s.serial(ctx, p.app, f.IdempotencyKey, func() reply {
		// Keep validation inside serialization. A retried idempotency key must
		// return its original response before inspecting a changed operation.
		if err := validateApplicationOperation(f.Op, f.IdempotencyKey); err != "" {
			return fail(err)
		}
		switch commandOperation(f.Op) {
		case operationScheduleSet:
			return s.schedules.Set(ctx, p.app, f)
		case operationScheduleCancel:
			return s.schedules.Cancel(ctx, p.app, f)
		case operationScheduleExtend:
			return s.schedules.Extend(ctx, p.app, f)
		case operationScheduleThrottle:
			return s.schedules.Throttle(ctx, p.app, f)
		case operationDeliveryAck:
			return s.ack(ctx, p.app, f)
		case operationWorkflowStart:
			return s.startWorkflow(ctx, p.app, f)
		case operationSignal:
			return s.signal(ctx, p.app, f)
		default:
			// validateApplicationOperation keeps this unreachable. Retaining the
			// protocol error guards future catalog/router drift.
			return fail("unknown op")
		}
	})
}
