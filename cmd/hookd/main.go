package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Ciggzy1312/hookd/internal/envfile"
	"github.com/Ciggzy1312/hookd/internal/server"
	"github.com/Ciggzy1312/hookd/internal/store"
)

func main() {
	env := envfile.Load(".env")
	maxDef := store.DefaultMax
	if v := env["MAX"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			maxDef = n
		}
	}

	addr := flag.String("addr", envDefault(env, "ADDR", "127.0.0.1:8080"), "listen address")
	max := flag.Int("max", maxDef, "max stored requests per inbox")
	replayURL := flag.String("replay-url", envDefault(env, "REPLAY_URL", ""), "default replay target URL")
	forward := flag.String("forward", envDefault(env, "FORWARD", ""), "reverse-proxy captured requests to this URL")
	flag.Parse()

	log := slog.Default()
	srv := server.New(server.Config{
		Addr:      *addr,
		Max:       *max,
		Log:       log,
		ReplayURL: *replayURL,
		Forward:   *forward,
	})

	writeBanner(os.Stdout, srv.BaseURL(), srv.InboxURL(), *forward)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "url", srv.BaseURL(), "inbox", srv.InboxURL(), "forward", *forward)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
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

func envDefault(env map[string]string, key, fallback string) string {
	if env != nil {
		if v := env[key]; v != "" {
			return v
		}
	}
	return fallback
}
