package server

import (
	"context"
	"github.com/bzrlab/calcron/internal/protocol"
)

func (s *Server) dashboardStats(ctx context.Context) reply { return s.deliveryAdmin.Stats(ctx) }
func (s *Server) replayDelivery(ctx context.Context, f frame) reply {
	return s.deliveryAdmin.Replay(ctx, protocol.Frame(f))
}
func (s *Server) cancelDelivery(ctx context.Context, f frame) reply {
	return s.deliveryAdmin.Cancel(ctx, protocol.Frame(f))
}
