// Payment service — receives order.paid and POSTs payment.succeeded
// to WEBHOOK_URL (the hookd inbox for the demo).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"uuid"

	"github.com/Ciggzy1312/hookd/internal/envfile"
)

type orderPaid struct {
	Type     string `json:"type"`
	OrderID  string `json:"order_id"`
	Item     string `json:"item"`
	Amount   int    `json:"amount"`
	Currency string `json:"currency"`
	PaidAt   string `json:"paid_at"`
}

type payment struct {
	ID        string `json:"id"`
	OrderID   string `json:"order_id"`
	Amount    int    `json:"amount"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type webhookEvent struct {
	Type    string  `json:"type"`
	ID      string  `json:"id"`
	Created int64   `json:"created"`
	Data    payment `json:"data"`
}

type server struct {
	log        *slog.Logger
	client     *http.Client
	webhookURL string
	mu         sync.Mutex
	payments   []payment
}

func main() {
	env := loadEnv()
	addr := envDefault(env, "ADDR", "127.0.0.1:3002")
	log := slog.Default()

	webhookURL, err := resolveWebhookURL(env)
	if err != nil {
		log.Error("webhook url", "err", err)
		os.Exit(1)
	}

	s := &server{
		log:        log,
		client:     &http.Client{Timeout: 10 * time.Second},
		webhookURL: webhookURL,
		payments:   []payment{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleInfo)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /payments", s.handleList)
	mux.HandleFunc("POST /events/order-paid", s.handleOrderPaid)

	httpSrv := &http.Server{Addr: addr, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "url", "http://"+addr+"/", "webhook", webhookURL)
		fmt.Fprintf(os.Stdout, "payment  http://%s/\n  webhook %s\n", addr, webhookURL)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Error("shutdown", "err", err)
			os.Exit(1)
		}
		log.Info("stopped")
	case err := <-errCh:
		if err != nil {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}
}

func (s *server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "payment",
		"webhook": s.webhookURL,
		"endpoints": map[string]string{
			"POST /events/order-paid": "receive order.paid, emit payment.succeeded webhook",
			"GET /payments":           "list payments",
		},
	})
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) handleList(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, slices.Clone(s.payments))
}

func (s *server) handleOrderPaid(w http.ResponseWriter, r *http.Request) {
	var ev orderPaid
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&ev); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if ev.OrderID == "" || ev.Amount <= 0 {
		http.Error(w, "order_id and positive amount required", http.StatusBadRequest)
		return
	}

	pay := payment{
		ID:        "pay_" + shortID(),
		OrderID:   ev.OrderID,
		Amount:    ev.Amount,
		Currency:  ev.Currency,
		Status:    "succeeded",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if pay.Currency == "" {
		pay.Currency = "usd"
	}

	s.mu.Lock()
	s.payments = append(s.payments, pay)
	s.mu.Unlock()

	s.log.Info("order.paid", "order", pay.OrderID, "payment", pay.ID, "amount", pay.Amount)

	go s.emitWebhook(pay)

	writeJSON(w, http.StatusAccepted, pay)
}

func (s *server) emitWebhook(pay payment) {
	ev := webhookEvent{
		Type:    "payment.succeeded",
		ID:      "evt_" + shortID(),
		Created: time.Now().Unix(),
		Data:    pay,
	}
	body, err := json.Marshal(ev)
	if err != nil {
		s.log.Error("webhook encode", "err", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		s.log.Error("webhook request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "hookd-demo-payment/1")
	req.Header.Set("X-Event-Type", "payment.succeeded")
	req.Header.Set("X-Webhook-Id", ev.ID)
	req.Header.Set("X-Order-Id", pay.OrderID)
	req.Header.Set("X-Payment-Id", pay.ID)

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Error("webhook send", "err", err, "url", s.webhookURL)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	s.log.Info("webhook sent", "event", ev.ID, "status", resp.StatusCode, "url", s.webhookURL)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func shortID() string {
	h := strings.ReplaceAll(uuid.NewV7().String(), "-", "")
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func loadEnv() map[string]string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return envfile.Load(".env")
	}
	return envfile.Load(filepath.Join(filepath.Dir(file), ".env"))
}

func envDefault(env map[string]string, key, fallback string) string {
	if env != nil {
		if v := env[key]; v != "" {
			return v
		}
	}
	return fallback
}

var inboxRe = regexp.MustCompile(`https?://[^"'<\s]+/i/[0-9a-fA-F-]+`)

func resolveWebhookURL(env map[string]string) (string, error) {
	if v := envDefault(env, "WEBHOOK_URL", ""); v != "" {
		return v, nil
	}
	base := envDefault(env, "HOOKD_URL", "http://127.0.0.1:8080")
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(base + "/")
	if err != nil {
		return "", fmt.Errorf("start hookd first, or set WEBHOOK_URL (could not reach %s: %w)", base, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", err
	}
	u := inboxRe.FindString(string(b))
	if u == "" {
		return "", fmt.Errorf("no inbox URL on %s — is hookd running?", base)
	}
	return u, nil
}
