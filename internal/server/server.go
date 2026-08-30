package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	json "encoding/json/v2"

	"github.com/Ciggzy1312/hookd/internal/store"
)

const maxBody = 1 << 20 // 1 MiB

type Config struct {
	Addr      string
	Max       int
	Store     *store.Store
	Log       *slog.Logger
	ReplayURL string
	Forward   string
	Client    *http.Client
}

type Server struct {
	http      *http.Server
	store     *store.Store
	hub       *Hub
	inboxID   string
	log       *slog.Logger
	replayURL string
	forward   *url.URL
	client    *http.Client
}

type captureResponse struct {
	OK bool   `json:"ok"`
	ID string `json:"id"`
}

func New(cfg Config) *Server {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	st := cfg.Store
	if st == nil {
		st = store.New(cfg.Max)
	}
	inboxID := st.Create()

	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	fwd := parseForward(cfg.Forward)
	if cfg.Forward != "" && fwd == nil {
		log.Error("invalid -forward URL", "url", cfg.Forward)
	}
	s := &Server{
		store:     st,
		hub:       newHub(),
		inboxID:   inboxID,
		log:       log,
		replayURL: cfg.ReplayURL,
		forward:   fwd,
		client:    client,
	}
	st.SetNotify(s.publish)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleLanding)
	mux.HandleFunc("GET /static/style.css", s.handleCSS)
	mux.HandleFunc("POST /inboxes", s.handleNewInbox)
	mux.HandleFunc("GET /i/{id}", s.handleInboxPage)
	mux.HandleFunc("GET /i/{id}/events", s.handleEvents)
	mux.HandleFunc("GET /i/{id}/requests", s.handleListRequests)
	mux.HandleFunc("GET /i/{id}/requests/{rid}", s.handleGetRequest)
	mux.HandleFunc("POST /i/{id}/replay", s.handleReplay)
	mux.HandleFunc("/i/{id}", s.handleCapture)

	s.http = &http.Server{
		Addr:    cfg.Addr,
		Handler: wrapHandler(log, mux),
	}
	return s
}

func (s *Server) Handler() http.Handler { return s.http.Handler }

func (s *Server) InboxID() string { return s.inboxID }

func (s *Server) Store() *store.Store { return s.store }

func (s *Server) BaseURL() string {
	return "http://" + s.http.Addr + "/"
}

func (s *Server) InboxURL() string {
	return "http://" + s.http.Addr + "/i/" + s.inboxID
}

func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	s.Close()
	return err
}

func (s *Server) Close() {
	s.hub.Stop()
}

func (s *Server) publish(rec store.Record) {
	b, err := json.Marshal(toRecordJSON(rec))
	if err != nil {
		s.log.Error("sse encode", "err", err)
		return
	}
	s.hub.Publish(rec.InboxID, b)
}

func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	writeCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	id := r.PathValue("id")
	if !s.store.Has(id) {
		http.NotFound(w, r)
		return
	}

	start := time.Now()
	body, trunc, err := readBody(r.Body, maxBody)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	rec, err := s.store.Append(id, store.Record{
		Method:     r.Method,
		Path:       r.URL.Path,
		Query:      r.URL.RawQuery,
		Headers:    r.Header,
		Body:       body,
		BodyTrunc:  trunc,
		RemoteAddr: r.RemoteAddr,
		Duration:   time.Since(start),
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if s.forward != nil {
		s.forwardCapture(w, r, rec, body)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.MarshalWrite(w, captureResponse{OK: true, ID: rec.ID}); err != nil {
		s.log.Error("encode response", "err", err)
	}
}

func readBody(r io.Reader, cap int) ([]byte, bool, error) {
	if r == nil {
		return nil, false, nil
	}
	body, err := io.ReadAll(io.LimitReader(r, int64(cap)+1))
	if err != nil {
		return nil, false, err
	}
	if len(body) > cap {
		return body[:cap], true, nil
	}
	return body, false, nil
}

func writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
}
