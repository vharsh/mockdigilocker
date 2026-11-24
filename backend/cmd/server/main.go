// Command server runs the Mock DigiLocker sandbox: DigiLocker-compatible
// OAuth/document APIs, consent pages, user portal, developer console and docs.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mockdigilocker/internal/config"
	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/server"
	"mockdigilocker/internal/store"
	"mockdigilocker/internal/webui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" { // for Docker HEALTHCHECK (distroless has no curl)
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		resp, err := http.Get("http://127.0.0.1:" + port + "/healthz")
		if err != nil || resp.StatusCode != 200 {
			os.Exit(1)
		}
		os.Exit(0)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var st store.Store
	switch cfg.Store {
	case "postgres":
		st, err = store.NewPostgres(cfg.DatabaseURL)
	default:
		st, err = store.NewMemory(cfg.DataFile)
	}
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer st.Close()

	var k kv.KV
	switch cfg.KV {
	case "redis":
		k, err = kv.NewRedis(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB)
		if err != nil {
			return fmt.Errorf("kv: %w", err)
		}
	default:
		k = kv.NewMemory()
	}
	defer k.Close()

	app, err := server.New(cfg, st, k, server.Options{UI: webui.FS(), Logger: log})
	if err != nil {
		return err
	}
	defer app.Close()

	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: app, ReadHeaderTimeout: 15 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		log.Info("Mock DigiLocker sandbox listening", "addr", srv.Addr, "public_base_url", cfg.PublicBaseURL,
			"store", cfg.Store, "kv", cfg.KV, "env", cfg.EnvName, "otp_mode", cfg.OTPMode)
		log.Info("open " + cfg.PublicBaseURL + " (portal) · " + cfg.PublicBaseURL + "/admin (console) · " + cfg.PublicBaseURL + "/docs (docs) · " + cfg.PublicBaseURL + "/demo-partner")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case <-sig:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
