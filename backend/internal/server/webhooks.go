package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"mockdigilocker/internal/model"
	"mockdigilocker/internal/scenario"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/store"
)

// Webhook event types.
var WebhookEventTypes = []string{
	"authentication.success", "authorization.failed", "consent.approved", "consent.denied",
	"consent.revoked", "token.issued", "document.retrieved", "document.pulled",
}

type WebhookAttempt struct {
	N          int       `json:"n"`
	At         time.Time `json:"at"`
	StatusCode int       `json:"status_code"`
	Error      string    `json:"error,omitempty"`
	Response   string    `json:"response,omitempty"`
	DurationMs int64     `json:"duration_ms"`
	Simulated  bool      `json:"simulated,omitempty"`
}

type WebhookEvent struct {
	ID          string           `json:"id"`
	Type        string           `json:"type"`
	ClientID    string           `json:"client_id"`
	UserID      string           `json:"user_id,omitempty"`
	URL         string           `json:"url"`
	Payload     json.RawMessage  `json:"payload"`
	Status      string           `json:"status"` // pending | retrying | delivered | failed
	Attempts    []WebhookAttempt `json:"attempts"`
	NextRetryAt *time.Time       `json:"next_retry_at,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// Dispatcher delivers signed webhook callbacks with exponential retries.
//
// Headers: X-MockDL-Event, X-MockDL-Delivery, X-MockDL-Timestamp and
// X-MockDL-Signature: sha256=<hex HMAC-SHA256(webhook_secret or client_secret, body)>.
type Dispatcher struct {
	a      *App
	client *http.Client
	mu     sync.Mutex
	timers map[string]*time.Timer
	closed bool
}

func NewDispatcher(a *App) *Dispatcher {
	return &Dispatcher{a: a, client: &http.Client{Timeout: a.cfg.WebhookTimeout}, timers: map[string]*time.Timer{}}
}

// Emit queues an event for the client if it has a callback URL and is
// subscribed to the event type.
func (d *Dispatcher) Emit(c *model.Client, typ, userID string, data map[string]any) *WebhookEvent {
	if c == nil || c.CallbackURL == "" {
		return nil
	}
	if len(c.WebhookEvents) > 0 && !slices.Contains(c.WebhookEvents, typ) {
		return nil
	}
	now := d.a.now().UTC()
	id := "evt_" + security.RandomHex(10)
	payload, _ := json.Marshal(map[string]any{"id": id, "type": typ, "created_at": now, "client_id": c.ClientID,
		"digilockerid": userID, "data": data, "sandbox": true})
	ev := &WebhookEvent{ID: id, Type: typ, ClientID: c.ClientID, UserID: userID, URL: c.CallbackURL, Payload: payload,
		Status: "pending", Attempts: []WebhookAttempt{}, CreatedAt: now, UpdatedAt: now}
	d.save(ev)
	go d.deliver(ev.ID)
	return ev
}

func (d *Dispatcher) save(ev *WebhookEvent) {
	ev.UpdatedAt = d.a.now().UTC()
	_ = d.a.st.Put(model.CollWebhooks, ev.ID, ev)
	_ = d.a.st.Trim(model.CollWebhooks, 2000)
}

// Redeliver triggers an immediate new delivery attempt.
func (d *Dispatcher) Redeliver(id string) (*WebhookEvent, error) {
	ev, err := store.GetT[WebhookEvent](d.a.st, model.CollWebhooks, id)
	if err != nil {
		return nil, err
	}
	ev.Status = "pending"
	ev.NextRetryAt = nil
	d.save(ev)
	go d.deliver(id)
	return ev, nil
}

func (d *Dispatcher) deliver(id string) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	delete(d.timers, id)
	d.mu.Unlock()

	ev, err := store.GetT[WebhookEvent](d.a.st, model.CollWebhooks, id)
	if err != nil {
		return
	}
	c := d.a.getClient(ev.ClientID)
	secret := ""
	if c != nil {
		secret = c.WebhookSecret
		if secret == "" {
			secret = c.ClientSecret
		}
	}
	att := WebhookAttempt{N: len(ev.Attempts) + 1, At: d.a.now().UTC()}
	s := d.a.selectScenario(nil, scenario.EPWebhook, ev.ClientID, ev.UserID)
	if s != nil && s.LatencyMs > 0 {
		time.Sleep(time.Duration(s.LatencyMs) * time.Millisecond)
	}
	start := time.Now()
	if s != nil && s.Effect == "webhook_failure" {
		att.Simulated = true
		att.Error = "simulated delivery failure (webhook_failure scenario)"
	} else {
		req, _ := http.NewRequest(http.MethodPost, ev.URL, bytes.NewReader(ev.Payload))
		ts := itoa(int(d.a.now().Unix()))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "MockDigiLocker-Webhooks/"+Version)
		req.Header.Set("X-MockDL-Event", ev.Type)
		req.Header.Set("X-MockDL-Delivery", ev.ID)
		req.Header.Set("X-MockDL-Timestamp", ts)
		req.Header.Set("X-MockDL-Signature", "sha256="+security.HMACHex(secret, ev.Payload))
		resp, err := d.client.Do(req)
		if err != nil {
			att.Error = err.Error()
		} else {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			att.StatusCode = resp.StatusCode
			att.Response = string(b)
		}
	}
	att.DurationMs = time.Since(start).Milliseconds()
	ev.Attempts = append(ev.Attempts, att)
	if att.StatusCode >= 200 && att.StatusCode < 300 {
		ev.Status = "delivered"
		ev.NextRetryAt = nil
	} else if len(ev.Attempts) >= d.a.cfg.WebhookMaxAttempts {
		ev.Status = "failed"
		ev.NextRetryAt = nil
	} else {
		ev.Status = "retrying"
		wait := d.a.cfg.WebhookBackoff * time.Duration(1<<(len(ev.Attempts)-1))
		next := d.a.now().UTC().Add(wait)
		ev.NextRetryAt = &next
		d.mu.Lock()
		if !d.closed {
			d.timers[id] = time.AfterFunc(wait, func() { d.deliver(id) })
		}
		d.mu.Unlock()
	}
	d.save(ev)
}

func (d *Dispatcher) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	for _, t := range d.timers {
		t.Stop()
	}
}
