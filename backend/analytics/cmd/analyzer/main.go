package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/aisleflow/backend/analytics/core"
	"example.com/aisleflow/backend/analytics/store"
	signaling "example.com/aisleflow/backend/analytics/temporal"
	"example.com/aisleflow/backend/analytics/transport"
	"example.com/aisleflow/backend/common/config"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		slog.Error("analyzer stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dsn, err := config.Required("DATABASE_URL")
	if err != nil {
		return err
	}
	raw, err := config.Required("TELEMETRY_TOKENS")
	if err != nil {
		return err
	}
	tokens, tenants, err := config.Tokens(raw)
	if err != nil {
		return err
	}
	cfg, err := config.Analyzer(config.Env("BASELINES_FILE", "config/baselines.json"))
	if err != nil {
		return err
	}
	startup, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	repo, err := store.Open(startup, dsn)
	if err != nil {
		return err
	}
	defer repo.DB.Close()
	a, err := core.New(cfg, repo)
	if err != nil {
		return err
	}
	tc, err := client.DialContext(startup, client.Options{HostPort: config.Env("TEMPORAL_ADDRESS", "localhost:7233")})
	if err != nil {
		return err
	}
	defer tc.Close()
	dispatcher := core.Dispatcher{Store: repo, Signaler: signaling.Signaler{Client: tc}, Tenants: tenants, MaxAttempts: 8}
	lis, err := net.Listen("tcp", config.Env("OTLP_LISTEN", ":4317"))
	if err != nil {
		return err
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(4<<20), grpc.MaxConcurrentStreams(32))
	collector.RegisterTraceServiceServer(server, &transport.Receiver{Analyzer: a, Tokens: tokens})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := repo.DB.PingContext(c); err != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		if _, err := tc.CheckHealth(c, &client.CheckHealthRequest{}); err != nil {
			http.Error(w, "temporal unavailable", 503)
			return
		}
		w.WriteHeader(200)
	})
	httpServer := &http.Server{Addr: config.Env("HTTP_LISTEN", ":8080"), Handler: mux, ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second}
	errs := make(chan error, 2)
	go func() { errs <- server.Serve(lis) }()
	go func() { errs <- httpServer.ListenAndServe() }()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c, stop := context.WithTimeout(ctx, 5*time.Second)
				if err := dispatcher.Dispatch(c); err != nil {
					slog.Warn("outbox delivery deferred", "error", err)
				}
				stop()
			}
		}
	}()
	slog.Info("analyzer ready", "otlp", lis.Addr(), "tenants", tenants)
	select {
	case <-ctx.Done():
	case err = <-errs:
	}
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	_ = httpServer.Shutdown(shutdown)
	grpcDone := make(chan struct{})
	go func() { server.GracefulStop(); close(grpcDone) }()
	select {
	case <-grpcDone:
	case <-shutdown.Done():
		server.Stop()
	}
	<-workerDone
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}
