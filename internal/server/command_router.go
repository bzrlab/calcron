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
		switch f.Op {
		case "app.create":
			return s.createApp(ctx, f)
		case "app.token.rotate":
			return s.rotateToken(ctx, f)
		case "app.token.revoke":
			return s.revokeToken(ctx, f)
		case "workflow.publish":
			return s.publishWorkflow(ctx, f)
		case "calendar.set":
			return s.setCalendar(ctx, f)
		case "calendar.next":
			return s.nextCalendar(ctx, f)
		case "start-schedule.set":
			return s.setStartSchedule(ctx, f)
		case "dashboard.stats":
			return s.dashboardStats(ctx)
		case "dashboard.list":
			return s.dashboardReads.list(ctx, f)
		case "delivery.replay":
			return s.replayDelivery(ctx, f)
		case "delivery.cancel":
			return s.cancelDelivery(ctx, f)
		default:
			return fail("unknown admin op")
		}
	}
	if f.IdempotencyKey == "" {
		return fail("idempotencyKey required")
	}
	return s.serial(ctx, p.app, f.IdempotencyKey, func() reply {
		switch f.Op {
		case "schedule.set":
			return s.set(ctx, p.app, f)
		case "schedule.cancel":
			return s.cancel(ctx, p.app, f)
		case "schedule.extend":
			return s.extend(ctx, p.app, f)
		case "schedule.throttle":
			return s.throttle(ctx, p.app, f)
		case "delivery.ack":
			return s.ack(ctx, p.app, f)
		case "workflow.start":
			return s.startWorkflow(ctx, p.app, f)
		case "signal":
			return s.signal(ctx, p.app, f)
		default:
			return fail("unknown op")
		}
	})
}
