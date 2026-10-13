package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/bzrlab/calcron/internal/application"
	"github.com/bzrlab/calcron/internal/calendar"
	"github.com/bzrlab/calcron/internal/delivery"
	"github.com/bzrlab/calcron/internal/schedule"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config contains the process-level dependencies for a Server.
type Config struct {
	DatabaseURL, AdminToken, Migration string
	RetryBase                          time.Duration
}

// Server coordinates command handling, asynchronous delivery, and workflows.
type Server struct {
	db             *pgxpool.Pool
	admin          string
	hub            *hub
	id             string
	retryBase      time.Duration
	commands       *commandRouter
	dashboardReads *dashboardRead
	workflows      *workflowEngine
	applications   *application.Module
	calendars      *calendar.Module
	schedules      *schedule.Module
	deliveryAdmin  *delivery.Admin
	callbackHTTP   *http.Client
	callbackWait   sync.WaitGroup
	cancel         context.CancelFunc
	done           chan struct{}
	closeOnce      sync.Once
}

func New(ctx context.Context, c Config) (*Server, error) {
	if c.DatabaseURL == "" || c.AdminToken == "" {
		return nil, errors.New("DATABASE_URL and CALCRON_ADMIN_TOKEN required")
	}
	db, err := pgxpool.New(ctx, c.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(ctx, c.Migration); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	s := &Server{db: db, admin: c.AdminToken, hub: &hub{apps: map[string]map[*peer]struct{}{}}, id: random(), retryBase: c.RetryBase}
	s.commands = newCommandRouter(s)
	s.dashboardReads = newDashboardRead(db, s.hub)
	s.workflows = newWorkflowEngine(s)
	s.applications = application.New(db, random)
	s.calendars = calendar.New(db)
	s.schedules = schedule.New(db, s.calendars, random, s.startWorkflow)
	s.deliveryAdmin = delivery.NewAdmin(db)
	s.callbackHTTP = &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	loopCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		s.loop(loopCtx)
	}()
	return s, nil
}

func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		<-s.done
		s.callbackWait.Wait()
		s.db.Close()
	})
}

func (s *Server) ListenAndServe(addr string) error { return http.ListenAndServe(addr, s.Handler()) }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /commands", s.httpCommand)
	mux.HandleFunc("/ws", s.ws)
	mux.HandleFunc("/", s.dashboard)
	return mux
}

func (s *Server) loop(ctx context.Context) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.makeDue(ctx)
			s.deliver(ctx)
			s.deliverCallbacks(ctx)
			s.workflows.wake(ctx)
			s.schedules.StartRecurring(ctx)
			s.cleanupExpiredThrottles(ctx)
		}
	}
}

func random() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Server) cleanupExpiredThrottles(ctx context.Context) {
	_, _ = s.db.Exec(ctx, `delete from throttles where until_at <= now()`)
}
