package server

// main is in cmd/server/main.go in the real project; this file shows the
// graceful-shutdown wiring described in ch38 as a runnable example.
// Tests use BuildRouter directly — they don't call Run.

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Run starts the server and blocks until SIGINT or SIGTERM is received.
func Run(addr, dbPath string, jwtSecret []byte) error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	db, err := openDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	srv := &http.Server{
		Addr:         addr,
		Handler:      BuildRouter(db, jwtSecret),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server started", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutdown initiated")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return srv.Shutdown(ctx)
}
