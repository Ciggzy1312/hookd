package server

import (
	"fmt"
	"net/http"
	"sync"
)

type client struct {
	inbox string
	ch    chan []byte
}

type pub struct {
	inbox   string
	payload []byte
}

type Hub struct {
	register   chan client
	unregister chan client
	publish    chan pub
	done       chan struct{}
	stop       sync.Once
}

func newHub() *Hub {
	h := &Hub{
		register:   make(chan client),
		unregister: make(chan client),
		publish:    make(chan pub, 16),
		done:       make(chan struct{}),
	}
	go h.loop()
	return h
}

func (h *Hub) Stop() {
	h.stop.Do(func() { close(h.done) })
}

func (h *Hub) Publish(inbox string, payload []byte) {
	select {
	case h.publish <- pub{inbox: inbox, payload: payload}:
	case <-h.done:
	}
}

func (h *Hub) add(c client) {
	select {
	case h.register <- c:
	case <-h.done:
		close(c.ch)
	}
}

func (h *Hub) remove(c client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

func (h *Hub) loop() {
	subs := make(map[string]map[chan []byte]struct{})
	closeAll := func() {
		for _, m := range subs {
			for ch := range m {
				close(ch)
			}
		}
		clear(subs)
	}
	for {
		select {
		case <-h.done:
			closeAll()
			return
		case c := <-h.register:
			m := subs[c.inbox]
			if m == nil {
				m = make(map[chan []byte]struct{})
				subs[c.inbox] = m
			}
			m[c.ch] = struct{}{}
		case c := <-h.unregister:
			if m, ok := subs[c.inbox]; ok {
				if _, ok := m[c.ch]; ok {
					delete(m, c.ch)
					close(c.ch)
				}
				if len(m) == 0 {
					delete(subs, c.inbox)
				}
			}
		case p := <-h.publish:
			for ch := range subs[p.inbox] {
				select {
				case ch <- p.payload:
				default:
				}
			}
		}
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.store.Has(id) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if err := http.NewResponseController(w).Flush(); err != nil {
		return
	}

	ch := make(chan []byte, 8)
	c := client{inbox: id, ch: ch}
	s.hub.add(c)
	defer s.hub.remove(c)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			if err := http.NewResponseController(w).Flush(); err != nil {
				return
			}
		}
	}
}
