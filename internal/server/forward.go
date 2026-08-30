package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/Ciggzy1312/hookd/internal/store"
)

func parseForward(raw string) *url.URL {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil
	}
	return u
}

func (s *Server) forwardCapture(w http.ResponseWriter, r *http.Request, rec store.Record, body []byte) {
	proxy := httputil.NewSingleHostReverseProxy(s.forward)
	if s.client != nil && s.client.Transport != nil {
		proxy.Transport = s.client.Transport
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		b, trunc, err := readBody(resp.Body, maxBody)
		if err != nil {
			return err
		}
		resp.Body = io.NopCloser(bytes.NewReader(b))
		resp.ContentLength = int64(len(b))
		resp.Header.Set("Content-Length", strconv.Itoa(len(b)))
		_, _ = s.store.SetForward(rec.InboxID, rec.ID, store.Upstream{
			Status:    resp.StatusCode,
			Body:      b,
			BodyTrunc: trunc,
		})
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		_, _ = s.store.SetForward(rec.InboxID, rec.ID, store.Upstream{Error: err.Error()})
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}
