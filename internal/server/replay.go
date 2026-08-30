package server

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"

	json "encoding/json/v2"

	"github.com/Ciggzy1312/hookd/internal/store"
)

type replayRequest struct {
	ID     string `json:"id"`
	Target string `json:"target"`
}

type replayResponse struct {
	OK      bool       `json:"ok"`
	Request recordJSON `json:"request"`
}

func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	inbox := r.PathValue("id")
	var req replayRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil || req.ID == "" {
		http.Error(w, "invalid replay request", http.StatusBadRequest)
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		target = s.replayURL
	}
	if err := checkReplayTarget(target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rec, err := s.store.Get(inbox, req.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	up := s.replay(r, target, rec)
	updated, err := s.store.SetReplay(inbox, rec.ID, up)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, replayResponse{OK: true, Request: toRecordJSON(updated)})
}

func (s *Server) replay(r *http.Request, target string, rec store.Record) store.Upstream {
	out, err := http.NewRequestWithContext(r.Context(), rec.Method, target, bytes.NewReader(rec.Body))
	if err != nil {
		return store.Upstream{Error: err.Error()}
	}
	copyReplayHeaders(out.Header, rec.Headers)
	resp, err := s.client.Do(out)
	if err != nil {
		return store.Upstream{Error: err.Error()}
	}
	defer resp.Body.Close()
	body, trunc, err := readBody(resp.Body, maxBody)
	if err != nil {
		return store.Upstream{Status: resp.StatusCode, Error: err.Error()}
	}
	return store.Upstream{Status: resp.StatusCode, Body: body, BodyTrunc: trunc}
}

func checkReplayTarget(target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errInvalidTarget
	}
	return nil
}

type targetError string

func (e targetError) Error() string { return string(e) }

const errInvalidTarget targetError = "invalid replay target"

func copyReplayHeaders(dst, src http.Header) {
	for k, vs := range src {
		if skipReplayHeader(k) {
			continue
		}
		dst[k] = append([]string(nil), vs...)
	}
}

func skipReplayHeader(k string) bool {
	switch http.CanonicalHeaderKey(k) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Te", "Trailer", "Transfer-Encoding", "Upgrade", "Host", "Content-Length":
		return true
	default:
		return false
	}
}
