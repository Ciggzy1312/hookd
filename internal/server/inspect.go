package server

import (
	"encoding/base64"
	json "encoding/json/v2"
	"html/template"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/Ciggzy1312/hookd/internal/store"
	"github.com/Ciggzy1312/hookd/internal/ui"
)

type recordJSON struct {
	ID         string              `json:"id"`
	InboxID    string              `json:"inbox_id"`
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	Query      string              `json:"query"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body,omitempty"`
	BodyBase64 string              `json:"body_base64,omitempty"`
	BodyTrunc  bool                `json:"body_trunc"`
	RemoteAddr string              `json:"remote_addr"`
	Duration   string              `json:"duration"`
	ReceivedAt time.Time           `json:"received_at"`
}

type listResponse struct {
	OK       bool         `json:"ok"`
	ID       string       `json:"id"`
	Requests []recordJSON `json:"requests"`
}

func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	data := ui.LandingData{
		InboxID:  s.inboxID,
		InboxURL: "http://" + r.Host + "/i/" + s.inboxID,
	}
	s.render(w, http.StatusOK, ui.Landing, data)
}

func (s *Server) handleNewInbox(w http.ResponseWriter, r *http.Request) {
	id := s.store.Create()
	http.Redirect(w, r, "/i/"+id, http.StatusSeeOther)
}

func (s *Server) handleInboxPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.store.Has(id) {
		http.NotFound(w, r)
		return
	}
	s.render(w, http.StatusOK, ui.Inbox, ui.InboxData{ID: id})
}

func (s *Server) handleListRequests(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	recs, err := s.store.List(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	out := listResponse{OK: true, ID: id, Requests: make([]recordJSON, 0, len(recs))}
	for i := range recs {
		out.Requests = append(out.Requests, toRecordJSON(recs[i]))
	}
	writeJSON(w, out)
}

func (s *Server) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rid := r.PathValue("rid")
	rec, err := s.store.Get(id, rid)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, toRecordJSON(rec))
}

func toRecordJSON(rec store.Record) recordJSON {
	out := recordJSON{
		ID:         rec.ID,
		InboxID:    rec.InboxID,
		Method:     rec.Method,
		Path:       rec.Path,
		Query:      rec.Query,
		Headers:    rec.Headers,
		BodyTrunc:  rec.BodyTrunc,
		RemoteAddr: rec.RemoteAddr,
		Duration:   rec.Duration.String(),
		ReceivedAt: rec.ReceivedAt,
	}
	if utf8.Valid(rec.Body) {
		out.Body = string(rec.Body)
	} else {
		out.BodyBase64 = base64.StdEncoding.EncodeToString(rec.Body)
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.MarshalWrite(w, v); err != nil {
		// headers already sent; nothing useful to do
	}
}

func (s *Server) render(w http.ResponseWriter, status int, tmpl *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := tmpl.Execute(w, data); err != nil {
		s.log.Error("template", "err", err)
	}
}
