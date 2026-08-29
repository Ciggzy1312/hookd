package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	json "encoding/json/v2"

	"github.com/Ciggzy1312/hookd/internal/store"
)

const maxBody = 1 << 20 // 1 MiB

type Config struct {
	Addr  string
	Max   int
	Store *store.Store
	Log   *slog.Logger
}

type Server struct {
	http    *http.Server
	store   *store.Store
	inboxID string
	log     *slog.Logger
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

	s := &Server{
		store:   st,
		inboxID: inboxID,
		log:     log,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleLanding)
	mux.HandleFunc("GET /static/style.css", s.handleCSS)
	mux.HandleFunc("POST /inboxes", s.handleNewInbox)
	mux.HandleFunc("GET /i/{id}", s.handleInboxPage)
	mux.HandleFunc("GET /i/{id}/requests", s.handleListRequests)
	mux.HandleFunc("GET /i/{id}/requests/{rid}", s.handleGetRequest)
	mux.HandleFunc("/i/{id}", s.handleCapture)

	s.http = &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
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
	return s.http.Shutdown(ctx)
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
