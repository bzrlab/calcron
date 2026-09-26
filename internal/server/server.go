package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	DatabaseURL, AdminToken, Migration string
	RetryBase                          time.Duration
}
type Server struct {
	db             *pgxpool.Pool
	admin          string
	hub            *hub
	id             string
	retryBase      time.Duration
	commands       *commandRouter
	dashboardReads *dashboardRead
	workflows      *workflowEngine
}
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
type frame struct {
	ID             string          `json:"id,omitempty"`
	Op             string          `json:"op"`
	Token          string          `json:"token,omitempty"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
	Key            string          `json:"key,omitempty"`
	Event          string          `json:"event,omitempty"`
	After          string          `json:"after,omitempty"`
	At             string          `json:"at,omitempty"`
	By             string          `json:"by,omitempty"`
	Cooldown       string          `json:"cooldown,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
	Chain          json.RawMessage `json:"chain,omitempty"`
	Name           string          `json:"name,omitempty"`
	Namespace      string          `json:"namespace,omitempty"`
	ApplicationID  string          `json:"applicationId,omitempty"`
	Workflow       string          `json:"workflow,omitempty"`
	Correlation    string          `json:"correlationKey,omitempty"`
	Calendar       string          `json:"calendar,omitempty"`
	LocalTime      string          `json:"localTime,omitempty"`
	MissedPolicy   string          `json:"missedPolicy,omitempty"`
	DeliveryID     string          `json:"deliveryId,omitempty"`
	TokenID        string          `json:"tokenId,omitempty"`
}
type reply struct {
	ID    string `json:"id,omitempty"`
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
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
	s.dashboardReads = newDashboardRead(db)
	s.workflows = newWorkflowEngine(s)
	go s.loop(ctx)
	return s, nil
}
func (s *Server) Close() { s.db.Close() }
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.Handler())
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/ws", s.ws)
	mux.HandleFunc("/", s.dashboard)
	return mux
}
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	p := &peer{conn: c}
	ctx := r.Context()
	_, b, err := c.Read(ctx)
	if err != nil {
		return
	}
	var a frame
	if json.Unmarshal(b, &a) != nil || a.Op != "auth" {
		return
	}
	app, admin, err := s.commands.authenticate(ctx, a.Token)
	if err != nil {
		_ = p.send(ctx, reply{ID: a.ID, Error: "unauthorized"})
		return
	}
	p.app, p.admin = app, admin
	if !admin {
		s.hub.add(p)
	}
	defer s.hub.remove(p)
	_ = p.send(ctx, reply{ID: a.ID, OK: true, Data: map[string]any{"application": app, "admin": admin}})
	for {
		_, b, err = c.Read(ctx)
		if err != nil {
			return
		}
		var f frame
		if err := json.Unmarshal(b, &f); err != nil {
			_ = p.send(ctx, reply{Error: "bad json"})
			continue
		}
		res := s.commands.handle(ctx, p, f)
		res.ID = f.ID
		_ = p.send(ctx, res)
	}
}
func (p *peer) send(ctx context.Context, v any) error {
	b, _ := json.Marshal(v)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.conn.Write(ctx, websocket.MessageText, b)
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
	out := make([]*peer, 0, len(h.apps[app]))
	for p := range h.apps[app] {
		out = append(out, p)
	}
	return out
}
func (h *hub) peer(app string) *peer {
	h.RLock()
	defer h.RUnlock()
	for p := range h.apps[app] {
		return p
	}
	return nil
}

func fail(e string) reply { return reply{Error: e} }
func ok(v any) reply      { return reply{OK: true, Data: v} }
func idempotent(ctx context.Context, q pgx.Row) (reply, bool) {
	var raw []byte
	err := q.Scan(&raw)
	if err != nil {
		return reply{}, false
	}
	var r reply
	return r, json.Unmarshal(raw, &r) == nil
}
func (s *Server) serial(ctx context.Context, app, key string, run func() reply) reply {
	c, err := s.db.Acquire(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer c.Release()
	lockKey := app + ":" + key
	if _, err = c.Exec(ctx, `select pg_advisory_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return fail(err.Error())
	}
	defer c.Exec(context.Background(), `select pg_advisory_unlock(hashtextextended($1,0))`, lockKey)
	if r, ok := idempotent(ctx, c.QueryRow(ctx, `select response from idempotency where application_id=$1 and key=$2`, app, key)); ok {
		return r
	}
	r := run()
	return r
}
func rememberTx(ctx context.Context, tx pgx.Tx, app, key string, r reply) error {
	b, _ := json.Marshal(r)
	_, err := tx.Exec(ctx, `insert into idempotency(application_id,key,response) values($1,$2,$3)`, app, key, b)
	return err
}
func (s *Server) createApp(ctx context.Context, f frame) reply {
	if f.Name == "" || f.Namespace == "" {
		return fail("name and namespace required")
	}
	id := random()
	tid := random()
	secret := random() + random()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return fail(err.Error())
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `insert into applications(id,namespace) values($1,$2)`, id, f.Namespace)
	if err == nil {
		_, err = tx.Exec(ctx, `insert into application_tokens(id,application_id,secret_hash) values($1,$2,$3)`, tid, id, string(hash))
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return fail("application exists")
	}
	return ok(map[string]string{"applicationId": id, "token": "cc_" + tid + "_" + secret})
}
func (s *Server) rotateToken(ctx context.Context, f frame) reply {
	if f.ApplicationID == "" {
		return fail("applicationId required")
	}
	tid, secret := random(), random()+random()
	hash, e := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if e != nil {
		return fail(e.Error())
	}
	tag, e := s.db.Exec(ctx, `insert into application_tokens(id,application_id,secret_hash) select $1,id,$2 from applications where id=$3`, tid, string(hash), f.ApplicationID)
	if e != nil {
		return fail(e.Error())
	}
	if tag.RowsAffected() != 1 {
		return fail("application not found")
	}
	return ok(map[string]string{"tokenId": tid, "token": "cc_" + tid + "_" + secret})
}
func (s *Server) revokeToken(ctx context.Context, f frame) reply {
	if f.TokenID == "" {
		return fail("tokenId required")
	}
	tag, e := s.db.Exec(ctx, `update application_tokens set revoked_at=now() where id=$1 and revoked_at is null`, f.TokenID)
	if e != nil {
		return fail(e.Error())
	}
	return ok(map[string]bool{"revoked": tag.RowsAffected() == 1})
}
func deadline(f frame) (time.Time, error) {
	if f.After != "" {
		d, e := time.ParseDuration(f.After)
		return time.Now().UTC().Add(d), e
	}
	if f.At != "" {
		return time.Parse(time.RFC3339, f.At)
	}
	return time.Time{}, errors.New("after or at required")
}
func (s *Server) set(ctx context.Context, app string, f frame) reply {
	if f.Key == "" || f.Event == "" {
		return fail("key and event required")
	}
	if _, e := parseChain(f.Chain); e != nil {
		return fail(e.Error())
	}
	at, e := deadline(f)
	if e != nil {
		return fail(e.Error())
	}
	data := f.Data
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	id := random()
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return fail(e.Error())
	}
	defer tx.Rollback(ctx)
	var sid string
	e = tx.QueryRow(ctx, `insert into schedules(id,application_id,schedule_key,event,payload,run_at,status,chain) values($1,$2,$3,$4,$5,$6,'scheduled',$7) on conflict(application_id,schedule_key) do update set event=excluded.event,payload=excluded.payload,run_at=excluded.run_at,status='scheduled',chain=excluded.chain,updated_at=now() returning id`, id, app, f.Key, f.Event, data, at, f.Chain).Scan(&sid)
	if e != nil {
		return fail(e.Error())
	}
	_, e = tx.Exec(ctx, `update deliveries set status='cancelled' where schedule_id=$1 and status='pending'`, sid)
	if e == nil {
		_, e = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event,data) values($2,'schedule',$1,'set',$3)`, sid, app, data)
	}
	if e != nil {
		return fail(e.Error())
	}
	r := ok(map[string]any{"scheduleId": sid, "runAt": at})
	if e = rememberTx(ctx, tx, app, f.IdempotencyKey, r); e != nil {
		return fail(e.Error())
	}
	if e = tx.Commit(ctx); e != nil {
		return fail(e.Error())
	}
	return r
}

type chain struct {
	Key   string          `json:"key"`
	Event string          `json:"event"`
	After string          `json:"after"`
	Data  json.RawMessage `json:"data"`
}

func parseChain(raw json.RawMessage) (chain, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return chain{}, nil
	}
	var c chain
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, errors.New("invalid chain")
	}
	if c.Key == "" || c.Event == "" || c.After == "" {
		return c, errors.New("chain key, event and after required")
	}
	d, err := time.ParseDuration(c.After)
	if err != nil || d < 0 {
		return c, errors.New("invalid chain after")
	}
	return c, nil
}
func (s *Server) cancel(ctx context.Context, app string, f frame) reply {
	if f.Key == "" {
		return fail("key required")
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return fail(e.Error())
	}
	defer tx.Rollback(ctx)
	var sid string
	e = tx.QueryRow(ctx, `update schedules set status='cancelled',updated_at=now() where application_id=$1 and schedule_key=$2 and status!='cancelled' returning id`, app, f.Key).Scan(&sid)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return fail(e.Error())
	}
	if sid != "" {
		_, e = tx.Exec(ctx, `update deliveries set status='cancelled' where schedule_id=$1 and status='pending'`, sid)
		if e == nil {
			_, e = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event) values($2,'schedule',$1,'cancelled')`, sid, app)
		}
		if e != nil {
			return fail(e.Error())
		}
	}
	r := ok(map[string]bool{"cancelled": sid != ""})
	if e = rememberTx(ctx, tx, app, f.IdempotencyKey, r); e != nil {
		return fail(e.Error())
	}
	if e = tx.Commit(ctx); e != nil {
		return fail(e.Error())
	}
	return r
}
func (s *Server) extend(ctx context.Context, app string, f frame) reply {
	if f.Key == "" || f.By == "" {
		return fail("key and by required")
	}
	d, e := time.ParseDuration(f.By)
	if e != nil {
		return fail(e.Error())
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return fail(e.Error())
	}
	defer tx.Rollback(ctx)
	var at time.Time
	e = tx.QueryRow(ctx, `update schedules set run_at=greatest(run_at,now())+$3::interval,status='scheduled',updated_at=now() where application_id=$1 and schedule_key=$2 and status!='cancelled' returning run_at`, app, f.Key, d.String()).Scan(&at)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return fail("schedule not found")
		}
		return fail(e.Error())
	}
	r := ok(map[string]any{"runAt": at})
	if e = rememberTx(ctx, tx, app, f.IdempotencyKey, r); e != nil {
		return fail(e.Error())
	}
	if e = tx.Commit(ctx); e != nil {
		return fail(e.Error())
	}
	return r
}
func (s *Server) throttle(ctx context.Context, app string, f frame) reply {
	if f.Key == "" || f.Event == "" || f.Cooldown == "" {
		return fail("key, event and cooldown required")
	}
	d, e := time.ParseDuration(f.Cooldown)
	if e != nil {
		return fail(e.Error())
	}
	if _, e := parseChain(f.Chain); e != nil {
		return fail(e.Error())
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return fail(e.Error())
	}
	defer tx.Rollback(ctx)
	var allowed bool
	e = tx.QueryRow(ctx, `insert into throttles(application_id,throttle_key,until_at) values($1,$2,now()+$3::interval) on conflict(application_id,throttle_key) do update set until_at=excluded.until_at where throttles.until_at<=now() returning true`, app, f.Key, d.String()).Scan(&allowed)
	if errors.Is(e, pgx.ErrNoRows) {
		r := ok(map[string]bool{"triggered": false})
		if e = rememberTx(ctx, tx, app, f.IdempotencyKey, r); e != nil {
			return fail(e.Error())
		}
		if e = tx.Commit(ctx); e != nil {
			return fail(e.Error())
		}
		return r
	}
	if e != nil {
		return fail(e.Error())
	}
	f.After = "0s"
	r := s.setTx(ctx, tx, app, f)
	if !r.OK {
		return r
	}
	r = ok(map[string]bool{"triggered": true})
	if e = rememberTx(ctx, tx, app, f.IdempotencyKey, r); e != nil {
		return fail(e.Error())
	}
	if e = tx.Commit(ctx); e != nil {
		return fail(e.Error())
	}
	return r
}
func (s *Server) setTx(ctx context.Context, tx pgx.Tx, app string, f frame) reply {
	data := f.Data
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	var sid string
	e := tx.QueryRow(ctx, `insert into schedules(id,application_id,schedule_key,event,payload,run_at,status,chain) values($1,$2,$3,$4,$5,now(),'scheduled',$6) on conflict(application_id,schedule_key) do update set event=excluded.event,payload=excluded.payload,run_at=excluded.run_at,status='scheduled',chain=excluded.chain,updated_at=now() returning id`, random(), app, f.Key, f.Event, data, f.Chain).Scan(&sid)
	if e != nil {
		return fail(e.Error())
	}
	_, e = tx.Exec(ctx, `update deliveries set status='cancelled' where schedule_id=$1 and status='pending'`, sid)
	if e != nil {
		return fail(e.Error())
	}
	return ok(nil)
}
func (s *Server) ack(ctx context.Context, app string, f frame) reply {
	if f.DeliveryID == "" {
		return fail("deliveryId required")
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return fail(e.Error())
	}
	defer tx.Rollback(ctx)
	var sid, workflowID, nextState *string
	e = tx.QueryRow(ctx, `update deliveries set status='acked',acked_at=now() where id=$1 and application_id=$2 and status='pending' returning schedule_id,workflow_instance_id,next_state`, f.DeliveryID, app).Scan(&sid, &workflowID, &nextState)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return ok(map[string]bool{"acked": false})
		}
		return fail(e.Error())
	}
	if sid != nil {
		var chain []byte
		if e = tx.QueryRow(ctx, `select coalesce(chain,'null'::jsonb) from schedules where id=$1`, *sid).Scan(&chain); e != nil {
			return fail(e.Error())
		}
		if c, err := parseChain(chain); err != nil {
			return fail(err.Error())
		} else if c.Key != "" {
			sf := frame{Key: c.Key, Event: c.Event, After: c.After, Data: c.Data}
			if !s.setTx(ctx, tx, app, sf).OK {
				return fail("chain failed")
			}
		}
	}
	if workflowID != nil && nextState != nil {
		_, e = tx.Exec(ctx, `update workflow_instances set current_state=$2,status='running',updated_at=now() where id=$1 and status='waiting_ack'`, *workflowID, *nextState)
		if e != nil {
			return fail(e.Error())
		}
	}
	_, e = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event) values($1,'delivery',$2,'acked')`, app, f.DeliveryID)
	if e != nil {
		return fail(e.Error())
	}
	r := ok(map[string]bool{"acked": true})
	if e = rememberTx(ctx, tx, app, f.IdempotencyKey, r); e != nil {
		return fail(e.Error())
	}
	if e = tx.Commit(ctx); e != nil {
		return fail(e.Error())
	}
	if workflowID != nil && nextState != nil {
		s.workflows.run(ctx, *workflowID)
	}
	return r
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
			s.workflows.wake(ctx)
			s.startRecurring(ctx)
		}
	}
}
func (s *Server) makeDue(ctx context.Context) {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `select id,application_id,event,payload from schedules where status='scheduled' and run_at<=now() order by run_at limit 100 for update skip locked`)
	if e != nil {
		return
	}
	type due struct {
		id, app, event string
		data           []byte
	}
	var ds []due
	for rows.Next() {
		var d due
		if rows.Scan(&d.id, &d.app, &d.event, &d.data) == nil {
			ds = append(ds, d)
		}
	}
	rows.Close()
	for _, d := range ds {
		_, e = tx.Exec(ctx, `update schedules set status='delivered',updated_at=now() where id=$1`, d.id)
		if e == nil {
			_, e = tx.Exec(ctx, `insert into deliveries(id,schedule_id,application_id,event,payload,status,next_attempt_at) values($2,$1,$3,$4,$5,'pending',now())`, d.id, random(), d.app, d.event, d.data)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `insert into history(application_id,subject_type,subject_id,event) values($2,'schedule',$1,'due')`, d.id, d.app)
		}
		if e != nil {
			return
		}
	}
	_ = tx.Commit(ctx)
}
func (s *Server) deliver(ctx context.Context) {
	rows, e := s.db.Query(ctx, `select id,application_id,coalesce(schedule_id,''),event,payload,attempts from deliveries where status='pending' and next_attempt_at<=now() order by next_attempt_at limit 100`)
	if e != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, app, sid, event string
		var data []byte
		var attempts int
		if rows.Scan(&id, &app, &sid, &event, &data, &attempts) != nil {
			continue
		}
		p := s.hub.peer(app)
		if p == nil {
			continue
		}
		var claimedID, claimedApp, claimedSchedule, claimedEvent string
		var claimedData []byte
		var claimedAttempts int
		e = s.db.QueryRow(ctx, `with candidate as (
  select id from deliveries where id=$1 and status='pending' and next_attempt_at<=now()
  and (locked_until is null or locked_until<=now()) for update skip locked
) update deliveries d set locked_until=now()+interval '15 seconds',lease_owner=$2
from candidate where d.id=candidate.id
returning d.id,d.application_id,coalesce(d.schedule_id,''),d.event,d.payload,d.attempts`, id, s.id).Scan(&claimedID, &claimedApp, &claimedSchedule, &claimedEvent, &claimedData, &claimedAttempts)
		if e != nil {
			continue
		}
		if claimedAttempts >= 5 {
			_, _ = s.db.Exec(ctx, `update deliveries set status='blocked',locked_until=null,lease_owner=null where id=$1 and lease_owner=$2`, claimedID, s.id)
			continue
		}
		if p.send(ctx, map[string]any{"op": "delivery", "deliveryId": claimedID, "scheduleId": claimedSchedule, "event": claimedEvent, "data": json.RawMessage(claimedData)}) != nil {
			_, _ = s.db.Exec(ctx, `update deliveries set locked_until=null,lease_owner=null where id=$1 and lease_owner=$2 and status='pending'`, claimedID, s.id)
			continue
		}
		base := s.retryBase
		if base == 0 {
			base = time.Second
		}
		delay := base * time.Duration(1<<claimedAttempts)
		_, _ = s.db.Exec(ctx, `update deliveries set attempts=attempts+1,last_sent_at=now(),next_attempt_at=now()+$2::interval,locked_until=null,lease_owner=null where id=$1 and lease_owner=$3 and status='pending'`, claimedID, delay.String(), s.id)
	}
}
func random() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		log.Panic(e)
	}
	return hex.EncodeToString(b)
}
