package server

import (
	"log/slog"
	"net/http"
)

func recoverMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if log != nil {
						log.Error("panic", "err", v)
					}
					http.Error(w, "internal error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func wrapHandler(log *slog.Logger, mux http.Handler) http.Handler {
	csrf := http.NewCrossOriginProtection()
	csrf.AddInsecureBypassPattern("/i/{id}")
	return recoverMiddleware(log)(csrf.Handler(mux))
}
