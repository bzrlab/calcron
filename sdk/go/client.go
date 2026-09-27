package cron

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type Client struct {
	conn                 *websocket.Conn
	mu                   sync.Mutex
	writeMu              sync.Mutex
	connMu               sync.RWMutex
	n                    uint64
	pending              map[string]chan response
	handlers             map[string]func(Event)
	url, token           string
	closed, reconnecting atomic.Bool
}
type response struct {
	ID         string          `json:"id"`
	OK         bool            `json:"ok"`
	Data       json.RawMessage `json:"data"`
	Error      string          `json:"error"`
	Op         string          `json:"op"`
	DeliveryID string          `json:"deliveryId"`
	Event      string          `json:"event"`
}
type Event struct {
	ID, Name string
	Data     json.RawMessage
	client   *Client
}

// ScheduleResult identifies a schedule and its RFC3339 deadline.
type ScheduleResult struct {
	ScheduleID string    `json:"scheduleId"`
	RunAt      time.Time `json:"runAt"`
}

// DeadlineResult is returned when an existing schedule is extended.
type DeadlineResult struct {
	RunAt time.Time `json:"runAt"`
}
type CancelResult struct {
	Cancelled bool `json:"cancelled"`
}
type ThrottleResult struct {
	Triggered bool `json:"triggered"`
}
type WorkflowResult struct {
	InstanceID string `json:"instanceId"`
}
type SignalResult struct {
	Matched int `json:"matched"`
}
type AckResult struct {
	Acked bool `json:"acked"`
}

type Schedule struct {
	Key, Event, After, At, IdempotencyKey string
	Data                                  any
	Chain                                 *Chain
}

// Chain creates a successor schedule after its parent delivery is acknowledged.
type Chain struct {
	Key   string `json:"key"`
	Event string `json:"event"`
	After string `json:"after"`
	Data  any    `json:"data,omitempty"`
}

// Throttle permits one immediate delivery for Key, then suppresses triggers for Cooldown.
type Throttle struct {
	Key, Event, Cooldown, IdempotencyKey string
	Data                                 any
	Chain                                *Chain
}

func Connect(ctx context.Context, url, token string) (*Client, error) {
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	if err = authenticate(ctx, c, token); err != nil {
		c.CloseNow()
		return nil, err
	}
	x := &Client{conn: c, url: url, token: token, pending: map[string]chan response{}, handlers: map[string]func(Event){}}
	go x.read(context.Background(), c)
	return x, nil
}
func authenticate(ctx context.Context, c *websocket.Conn, token string) error {
	b, _ := json.Marshal(map[string]string{"op": "auth", "token": token})
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		return err
	}
	_, b, err := c.Read(ctx)
	if err != nil {
		return err
	}
	var r response
	if err = json.Unmarshal(b, &r); err != nil {
		return err
	}
	if !r.OK {
		return errors.New(r.Error)
	}
	return nil
}
func (c *Client) Close() error {
	c.closed.Store(true)
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn.Close(websocket.StatusNormalClosure, "")
}
func (c *Client) On(event string, fn func(Event)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[event] = fn
}
func (e Event) Ack(ctx context.Context) (AckResult, error) {
	return requestAs[AckResult](e.client, ctx, map[string]any{"op": "delivery.ack", "deliveryId": e.ID, "idempotencyKey": "ack:" + e.ID})
}
func (c *Client) Set(ctx context.Context, s Schedule) (ScheduleResult, error) {
	if (s.After == "") == (s.At == "") {
		return ScheduleResult{}, errors.New("exactly one of after or at required")
	}
	return requestAs[ScheduleResult](c, ctx, map[string]any{"op": "schedule.set", "key": s.Key, "event": s.Event, "after": s.After, "at": s.At, "data": s.Data, "chain": s.Chain, "idempotencyKey": s.IdempotencyKey})
}
func (c *Client) Throttle(ctx context.Context, t Throttle) (ThrottleResult, error) {
	return requestAs[ThrottleResult](c, ctx, map[string]any{"op": "schedule.throttle", "key": t.Key, "event": t.Event, "cooldown": t.Cooldown, "data": t.Data, "chain": t.Chain, "idempotencyKey": t.IdempotencyKey})
}
func (c *Client) Cancel(ctx context.Context, key, idempotencyKey string) (CancelResult, error) {
	return requestAs[CancelResult](c, ctx, map[string]any{"op": "schedule.cancel", "key": key, "idempotencyKey": idempotencyKey})
}
func (c *Client) Extend(ctx context.Context, key, by, idempotencyKey string) (DeadlineResult, error) {
	return requestAs[DeadlineResult](c, ctx, map[string]any{"op": "schedule.extend", "key": key, "by": by, "idempotencyKey": idempotencyKey})
}
func (c *Client) Start(ctx context.Context, name, idempotencyKey string, data any) (WorkflowResult, error) {
	return requestAs[WorkflowResult](c, ctx, map[string]any{"op": "workflow.start", "name": name, "data": data, "idempotencyKey": idempotencyKey})
}
func (c *Client) Signal(ctx context.Context, event, correlationKey, idempotencyKey string, data any) (SignalResult, error) {
	return requestAs[SignalResult](c, ctx, map[string]any{"op": "signal", "event": event, "correlationKey": correlationKey, "data": data, "idempotencyKey": idempotencyKey})
}
func requestAs[T any](c *Client, ctx context.Context, v map[string]any) (T, error) {
	var out T
	raw, err := c.request(ctx, v)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	return out, nil
}
func (c *Client) request(ctx context.Context, v map[string]any) (json.RawMessage, error) {
	id := itoa(atomic.AddUint64(&c.n, 1))
	v["id"] = id
	b, _ := json.Marshal(v)
	ch := make(chan response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	c.writeMu.Lock()
	c.connMu.RLock()
	conn := c.conn
	err := conn.Write(ctx, websocket.MessageText, b)
	c.connMu.RUnlock()
	c.writeMu.Unlock()
	if err != nil {
		c.remove(id)
		return nil, err
	}
	select {
	case r := <-ch:
		if !r.OK {
			return nil, errors.New(r.Error)
		}
		return r.Data, nil
	case <-ctx.Done():
		c.remove(id)
		return nil, ctx.Err()
	}
}
func (c *Client) read(ctx context.Context, conn *websocket.Conn) {
	for {
		_, b, err := conn.Read(ctx)
		if err != nil {
			c.failPending(err)
			if !c.closed.Load() {
				go c.reconnect()
			}
			return
		}
		var r response
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		if r.Op == "delivery" {
			c.mu.Lock()
			fn := c.handlers[r.Event]
			c.mu.Unlock()
			if fn != nil {
				go fn(Event{ID: r.DeliveryID, Name: r.Event, Data: r.Data, client: c})
			}
			continue
		}
		c.mu.Lock()
		ch := c.pending[r.ID]
		delete(c.pending, r.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- r
		}
	}
}
func (c *Client) reconnect() {
	if !c.reconnecting.CompareAndSwap(false, true) {
		return
	}
	defer c.reconnecting.Store(false)
	for delay := time.Second; !c.closed.Load(); delay = min(delay*2, 10*time.Second) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, _, err := websocket.Dial(ctx, c.url, nil)
		if err == nil {
			err = authenticate(ctx, conn, c.token)
			cancel()
			if err == nil {
				c.connMu.Lock()
				c.conn = conn
				c.connMu.Unlock()
				go c.read(context.Background(), conn)
				return
			}
			conn.CloseNow()
		} else {
			cancel()
		}
		time.Sleep(delay)
	}
}
func (c *Client) remove(id string) { c.mu.Lock(); defer c.mu.Unlock(); delete(c.pending, id) }
func (c *Client) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- response{Error: err.Error()}
	}
}
func itoa(n uint64) string {
	b := make([]byte, 0, 20)
	for {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
		if n == 0 {
			return string(b)
		}
	}
}
