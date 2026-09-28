package server

import (
	"context"

	"github.com/calcron/calcron/internal/dashboard"
	"github.com/calcron/calcron/internal/protocol"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dashboardRead struct{ module *dashboard.Module }

func newDashboardRead(db *pgxpool.Pool, hub *hub) *dashboardRead {
	return &dashboardRead{module: dashboard.New(db, hubPresence{hub})}
}

func (r *dashboardRead) list(ctx context.Context, f frame) reply {
	return r.module.List(ctx, protocol.Frame(f))
}

type hubPresence struct{ hub *hub }

func (p hubPresence) ConnectedApps() []string { return p.hub.connected() }
