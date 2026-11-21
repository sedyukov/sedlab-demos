package main

import (
	"sync"
	"time"
)

// Event is one line of the live log. Every step of the pipeline emits one or more events.
type Event struct {
	Seq      int64          `json:"seq"`
	TS       string         `json:"ts"`
	LeadID   string         `json:"lead_id,omitempty"`
	Step     string         `json:"step"`    // form, crm.upsert_contact, sheets.append_rows, webhook.payment, ...
	Service  string         `json:"service"` // form | crm | sheets | notify | payments | webhook | guard | system
	Status   string         `json:"status"`  // start | ok | retry | error | duplicate | rejected | info
	Attempt  int            `json:"attempt,omitempty"`
	Duration int64          `json:"duration_ms,omitempty"`
	Provider string         `json:"provider,omitempty"` // amoCRM (mock), Stripe (live), ...
	Message  string         `json:"message"`
	Data     map[string]any `json:"data,omitempty"`
}

const eventBacklog = 300

type sessionLog struct {
	events []Event
	subs   map[chan Event]struct{}
}

// Hub keeps a bounded backlog per session and fans events out to SSE subscribers.
type Hub struct {
	mu   sync.Mutex
	seq  int64
	logs map[string]*sessionLog
	now  func() time.Time
}

func NewHub() *Hub { return &Hub{logs: map[string]*sessionLog{}, now: time.Now} }

func (h *Hub) log(sid string) *sessionLog {
	l := h.logs[sid]
	if l == nil {
		l = &sessionLog{subs: map[chan Event]struct{}{}}
		h.logs[sid] = l
	}
	return l
}

// Publish stores the event and pushes it to subscribers (slow subscribers drop events, never block).
func (h *Hub) Publish(sid string, ev Event) Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	ev.Seq = h.seq
	ev.TS = h.now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	l := h.log(sid)
	l.events = append(l.events, ev)
	if len(l.events) > eventBacklog {
		l.events = append([]Event(nil), l.events[len(l.events)-eventBacklog:]...)
	}
	for ch := range l.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	return ev
}

// Subscribe returns the backlog after `after` and a live channel; call cancel when done.
func (h *Hub) Subscribe(sid string, after int64) ([]Event, <-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	l := h.log(sid)
	var backlog []Event
	for _, e := range l.events {
		if e.Seq > after {
			backlog = append(backlog, e)
		}
	}
	ch := make(chan Event, 64)
	l.subs[ch] = struct{}{}
	return backlog, ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := l.subs[ch]; ok {
			delete(l.subs, ch)
			close(ch)
		}
	}
}

// Drop forgets a session's log; open streams get closed.
func (h *Hub) Drop(sid string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if l := h.logs[sid]; l != nil {
		for ch := range l.subs {
			close(ch)
			delete(l.subs, ch) // cancel() of that stream becomes a no-op
		}
		delete(h.logs, sid)
	}
}

// Events returns a copy of the backlog (tests, snapshot API).
func (h *Hub) Events(sid string) []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	if l := h.logs[sid]; l != nil {
		return append([]Event(nil), l.events...)
	}
	return nil
}
