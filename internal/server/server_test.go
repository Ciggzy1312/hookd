package server

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	json "encoding/json/v2"

	"github.com/Ciggzy1312/hookd/internal/store"
)

func TestCaptureTable(t *testing.T) {
	st := store.New(32)
	srv := New(Config{Store: st, Max: 32})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	inbox := srv.InboxID()
	base := ts.URL + "/i/" + inbox

	tests := []struct {
		name    string
		method  string
		url     string
		headers http.Header
		body    []byte
		check   func(t *testing.T, rec store.Record)
	}{
		{
			name:   "headers",
			method: http.MethodPost,
			url:    base,
			headers: http.Header{
				"X-Test-Hook":  []string{"stripe"},
				"Content-Type": []string{"text/plain"},
			},
			body: []byte("hello"),
			check: func(t *testing.T, rec store.Record) {
				if rec.Headers.Get("X-Test-Hook") != "stripe" {
					t.Fatalf("X-Test-Hook = %q", rec.Headers.Get("X-Test-Hook"))
				}
				if rec.Method != http.MethodPost {
					t.Fatalf("method = %s", rec.Method)
				}
			},
		},
		{
			name:   "query",
			method: http.MethodPost,
			url:    base + "?foo=bar&x=1",
			body:   []byte("{}"),
			check: func(t *testing.T, rec store.Record) {
				if rec.Query != "foo=bar&x=1" {
					t.Fatalf("query = %q", rec.Query)
				}
			},
		},
		{
			name:   "json body",
			method: http.MethodPost,
			url:    base,
			headers: http.Header{
				"Content-Type": []string{"application/json"},
			},
			body: []byte(`{"hi":1}`),
			check: func(t *testing.T, rec store.Record) {
				if !bytes.Equal(rec.Body, []byte(`{"hi":1}`)) {
					t.Fatalf("body = %q", rec.Body)
				}
			},
		},
		{
			name:   "binary body",
			method: http.MethodPut,
			url:    base,
			body:   []byte{0x00, 0x01, 0xff},
			check: func(t *testing.T, rec store.Record) {
				if !bytes.Equal(rec.Body, []byte{0x00, 0x01, 0xff}) {
					t.Fatalf("body = %v", rec.Body)
				}
				if rec.Method != http.MethodPut {
					t.Fatalf("method = %s", rec.Method)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.url, bytes.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			for k, vs := range tt.headers {
				for _, v := range vs {
					req.Header.Add(k, v)
				}
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
				t.Fatal("missing CORS header")
			}
			var out captureResponse
			if err := json.UnmarshalRead(resp.Body, &out); err != nil {
				t.Fatal(err)
			}
			if !out.OK || out.ID == "" {
				t.Fatalf("response = %+v", out)
			}
			rec, err := st.Get(inbox, out.ID)
			if err != nil {
				t.Fatal(err)
			}
			if rec.Path != "/i/"+inbox {
				t.Fatalf("path = %q", rec.Path)
			}
			if rec.RemoteAddr == "" {
				t.Fatal("empty remote addr")
			}
			tt.check(t, rec)
		})
	}
}

func TestInspectAfterCapture(t *testing.T) {
	st := store.New(8)
	srv := New(Config{Store: st, Max: 8})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	inbox := srv.InboxID()
	resp, err := http.Post(ts.URL+"/i/"+inbox, "application/json", strings.NewReader(`{"hi":1}`))
	if err != nil {
		t.Fatal(err)
	}
	var cap captureResponse
	if err := json.UnmarshalRead(resp.Body, &cap); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	listResp, err := http.Get(ts.URL + "/i/" + inbox + "/requests")
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", listResp.StatusCode)
	}
	var list listResponse
	if err := json.UnmarshalRead(listResp.Body, &list); err != nil {
		t.Fatal(err)
	}
	if !list.OK || list.ID != inbox || len(list.Requests) != 1 {
		t.Fatalf("list = %+v", list)
	}
	if list.Requests[0].ID != cap.ID || list.Requests[0].Body != `{"hi":1}` {
		t.Fatalf("record = %+v", list.Requests[0])
	}

	one, err := http.Get(ts.URL + "/i/" + inbox + "/requests/" + cap.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Body.Close()
	var rec recordJSON
	if err := json.UnmarshalRead(one.Body, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.ID != cap.ID || rec.Method != http.MethodPost {
		t.Fatalf("one = %+v", rec)
	}
}

func TestInboxPageAndLanding(t *testing.T) {
	srv := New(Config{Max: 8})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	home, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(home.Body)
	home.Body.Close()
	if home.StatusCode != http.StatusOK {
		t.Fatalf("home status = %d", home.StatusCode)
	}
	if ct := home.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("home content-type = %q", ct)
	}
	if !strings.Contains(string(body), srv.InboxID()) {
		t.Fatalf("landing missing inbox id: %s", body)
	}
	if !strings.Contains(string(body), `/static/style.css`) {
		t.Fatal("landing missing stylesheet link")
	}

	css, err := http.Get(ts.URL + "/static/style.css")
	if err != nil {
		t.Fatal(err)
	}
	cssBody, _ := io.ReadAll(css.Body)
	css.Body.Close()
	if css.StatusCode != http.StatusOK {
		t.Fatalf("css status = %d", css.StatusCode)
	}
	if ct := css.Header.Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Fatalf("css content-type = %q", ct)
	}
	if !strings.Contains(string(cssBody), ":root") {
		t.Fatalf("css body: %s", cssBody)
	}

	page, err := http.Get(ts.URL + "/i/" + srv.InboxID())
	if err != nil {
		t.Fatal(err)
	}
	pageBody, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if page.StatusCode != http.StatusOK {
		t.Fatalf("inbox status = %d", page.StatusCode)
	}
	if !strings.Contains(string(pageBody), srv.InboxID()) {
		t.Fatalf("inbox page missing id: %s", pageBody)
	}
	for _, want := range []string{`data-inbox=`, `id="list"`, `data-tab`, "EventSource", "headers", "json", "raw", "hex"} {
		if !strings.Contains(string(pageBody), want) {
			t.Fatalf("inbox page missing %q", want)
		}
	}

	missing, err := http.Get(ts.URL + "/i/not-an-inbox")
	if err != nil {
		t.Fatal(err)
	}
	missBody, _ := io.ReadAll(missing.Body)
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing inbox status = %d", missing.StatusCode)
	}
	if ct := missing.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("404 content-type = %q", ct)
	}
	if !strings.Contains(string(missBody), "not found") {
		t.Fatalf("404 page: %s", missBody)
	}
}

func TestNewInbox(t *testing.T) {
	st := store.New(8)
	srv := New(Config{Store: st, Max: 8})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Post(ts.URL+"/inboxes", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	id, ok := strings.CutPrefix(loc, "/i/")
	if !ok || !st.Has(id) {
		t.Fatalf("Location = %q", loc)
	}
	if id == srv.InboxID() {
		t.Fatal("new inbox reused startup id")
	}
}

func TestInspectUnknown(t *testing.T) {
	srv := New(Config{Max: 8})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/i/not-an-inbox/requests")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestCaptureUnknownInbox(t *testing.T) {
	srv := New(Config{Max: 8})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Post(ts.URL+"/i/not-an-inbox", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestCaptureBodyCap(t *testing.T) {
	st := store.New(8)
	srv := New(Config{Store: st, Max: 8})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	body := bytes.Repeat([]byte("a"), maxBody+64)
	resp, err := http.Post(ts.URL+"/i/"+srv.InboxID(), "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out captureResponse
	if err := json.UnmarshalRead(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	rec, err := st.Get(srv.InboxID(), out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.BodyTrunc {
		t.Fatal("expected truncated body")
	}
	if len(rec.Body) != maxBody {
		t.Fatalf("len = %d, want %d", len(rec.Body), maxBody)
	}
}

func TestCaptureConcurrent(t *testing.T) {
	const n = 32
	st := store.New(n)
	srv := New(Config{Store: st, Max: n})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	url := ts.URL + "/i/" + srv.InboxID()
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			resp, err := http.Post(url, "text/plain", strings.NewReader(strings.Repeat("x", i+1)))
			if err != nil {
				t.Errorf("post: %v", err)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d", resp.StatusCode)
			}
		}(i)
	}
	wg.Wait()

	got, err := st.List(srv.InboxID())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("len = %d, want %d", len(got), n)
	}
}

func TestSSEReceivesCapture(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := New(Config{Max: 8})
		t.Cleanup(srv.Close)
		ts := httptest.NewTestServer(t, srv.Handler())
		client := ts.Client()
		inbox := srv.InboxID()

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		got := make(chan recordJSON, 1)
		go func() {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://hookd.test/i/"+inbox+"/events", nil)
			if err != nil {
				t.Error(err)
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("sse status = %d", resp.StatusCode)
				return
			}
			sc := bufio.NewScanner(resp.Body)
			for sc.Scan() {
				line := sc.Text()
				if rest, ok := strings.CutPrefix(line, "data: "); ok && rest != "" {
					var rec recordJSON
					if err := json.Unmarshal([]byte(rest), &rec); err != nil {
						t.Error(err)
						return
					}
					got <- rec
					return
				}
			}
			if err := sc.Err(); err != nil && ctx.Err() == nil {
				t.Error(err)
			}
		}()

		synctest.Sleep(0)

		resp, err := client.Post("http://hookd.test/i/"+inbox, "application/json", strings.NewReader(`{"sse":true}`))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("capture status = %d", resp.StatusCode)
		}

		synctest.Sleep(0)

		select {
		case rec := <-got:
			if rec.Body != `{"sse":true}` || rec.Method != http.MethodPost {
				t.Fatalf("event = %+v", rec)
			}
		default:
			t.Fatal("no SSE event")
		}
		cancel()
		synctest.Sleep(0)
	})
}

func TestCSRF(t *testing.T) {
	srv := New(Config{Max: 8})
	t.Cleanup(srv.Close)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	cross := func(method, url string) *http.Request {
		req, err := http.NewRequest(method, url, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	blocked, err := http.DefaultClient.Do(cross(http.MethodPost, ts.URL+"/inboxes"))
	if err != nil {
		t.Fatal(err)
	}
	blocked.Body.Close()
	if blocked.StatusCode != http.StatusForbidden {
		t.Fatalf("create inbox status = %d, want 403", blocked.StatusCode)
	}

	allowed, err := http.DefaultClient.Do(cross(http.MethodPost, ts.URL+"/i/"+srv.InboxID()))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, allowed.Body)
	allowed.Body.Close()
	if allowed.StatusCode != http.StatusOK {
		t.Fatalf("capture status = %d, want 200", allowed.StatusCode)
	}
}

func TestEventsUnknownInbox(t *testing.T) {
	srv := New(Config{Max: 8})
	t.Cleanup(srv.Close)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/i/not-an-inbox/events")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
