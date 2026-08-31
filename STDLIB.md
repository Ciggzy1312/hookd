# STDLIB.md
hookd is **Track C**. 
Runtime dependencies: **none**. 
`go.mod` has no `require` block. `golang.org/x` is not used.

## Package Killer

The featured kill is **`google/uuid` + a mux**.

Go 1.27 ships `package uuid` (RFC 9562). Inbox and record IDs are `uuid.NewV7()` — time-ordered, comparable `[16]byte`, no `github.com/google/uuid`.

Routing is `net/http` ServeMux patterns (1.22+). There is no chi and no gorilla/mux.

```go
mux.HandleFunc("GET /i/{id}", s.handleInboxPage)   // browser inspector
mux.HandleFunc("GET /i/{id}/events", s.handleEvents)
mux.HandleFunc("POST /i/{id}/replay", s.handleReplay)
mux.HandleFunc("/i/{id}", s.handleCapture)         // Stripe/GitHub POST
```

More-specific patterns win. `GET /{$}` is the landing page (a plain `GET /` would be a prefix catch-all and conflict with `/i/{id}`).

This is not a nodemon-class npm kill. We did not replace a file watcher; we deleted the two libraries a webhook inspector would actually import on day one.

## Substitutions

One line each. Every row is a real call site in this repo.

| Would have used | Stdlib (or 10-line stand-in) | Why |
|---|---|---|
| `github.com/google/uuid` | `uuid.NewV7()` | Inbox and record IDs; time-sortable, no module. |
| `chi` / `gorilla/mux` | ServeMux `GET /i/{id}`, `/i/{id}` | Method + path patterns; more specific wins. |
| `gorilla/csrf` | `http.NewCrossOriginProtection` | Fetch-metadata CSRF; bypass only `/i/{id}` for vendors. |
| `json-iterator` | `encoding/json/v2` | `MarshalWrite` / `UnmarshalRead` on HTTP bodies. |
| `logrus` / `zap` | `log/slog` | Structured listen/shutdown/panic logs. |
| `testify` | `testing` + `testing/synctest` | Table tests; SSE via `synctest.Test` + `httptest.NewTestServer` + `synctest.Sleep`. |
| `cobra` | `flag` | `-addr`, `-max`, `-forward`, `-replay-url`. |
| `rs/cors` | `Access-Control-Allow-*` headers | Capture must be callable from a browser `fetch`. |
| `joho/godotenv` | `internal/envfile` (~15 lines) | `KEY=VALUE`, `#` comments; overlays ADDR/FORWARD/REPLAY_URL/MAX. |
| `gorilla/handlers` recovery | `defer recover` middleware | Panic in a handler becomes 500, not a crashed process. |
| `net/http/httputil` (not a dep) | `NewSingleHostReverseProxy` | `--forward` is stdlib, not a custom hop client. |


## Reproducible build

`make reproduce` builds twice with `CGO_ENABLED=0`, `-trimpath`, `-ldflags="-buildid="`, prints both SHA-256s, and `cmp`s the binaries. That bonus is realistic in Go; we did not invent a lockfile theater.
