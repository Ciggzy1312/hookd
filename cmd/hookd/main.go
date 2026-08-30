package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ciggzy1312/hookd/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	max := flag.Int("max", 500, "max stored requests per inbox")
	replayURL := flag.String("replay-url", "", "default replay target URL")
	forward := flag.String("forward", "", "reverse-proxy captured requests to this URL")
	flag.Parse()

	log := slog.Default()
	srv := server.New(server.Config{
		Addr:      *addr,
		Max:       *max,
		Log:       log,
		ReplayURL: *replayURL,
		Forward:   *forward,
	})

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
