package metricsutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	vmmetrics "github.com/VictoriaMetrics/metrics"
)

const DefaultPath = "/metrics"

type Config struct {
	ListenAddr string
	Path       string
}

type Server struct {
	cfg      Config
	listener net.Listener
	server   *http.Server
}

func (c Config) Normalized() (Config, error) {
	cfg := c
	cfg.ListenAddr = strings.TrimSpace(cfg.ListenAddr)
	cfg.Path = strings.TrimSpace(cfg.Path)
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	if !strings.HasPrefix(cfg.Path, "/") {
		return cfg, fmt.Errorf("metrics path %q must start with /", cfg.Path)
	}

	return cfg, nil
}

func NewHandler(cfg Config, sets ...*vmmetrics.Set) (http.Handler, Config, error) {
	normalizedCfg, err := cfg.Normalized()
	if err != nil {
		return nil, normalizedCfg, err
	}

	vmmetrics.ExposeMetadata(true)

	mux := http.NewServeMux()
	mux.HandleFunc(normalizedCfg.Path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		for _, set := range sets {
			if set != nil {
				set.WritePrometheus(w)
			}
		}
		vmmetrics.WritePrometheus(w, true)
	})

	return mux, normalizedCfg, nil
}

func Start(ctx context.Context, logger *slog.Logger, cfg Config, sets ...*vmmetrics.Set) (*Server, error) {
	normalizedCfg, err := cfg.Normalized()
	if err != nil {
		return nil, err
	}
	if normalizedCfg.ListenAddr == "" {
		return nil, nil
	}

	handler, normalizedCfg, err := NewHandler(normalizedCfg, sets...)
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", normalizedCfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen metrics on %q: %w", normalizedCfg.ListenAddr, err)
	}

	logger = normalizeLogger(logger)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s := &Server{
		cfg:      normalizedCfg,
		listener: listener,
		server:   server,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.Close(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server shutdown failed", "addr", s.Addr(), "path", normalizedCfg.Path, "err", err)
		}
	}()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server failed", "addr", listener.Addr().String(), "path", normalizedCfg.Path, "err", err)
		}
	}()

	logger.Info("metrics server listening", "addr", listener.Addr().String(), "path", normalizedCfg.Path)

	return s, nil
}

func (s *Server) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}

	return s.listener.Addr().String()
}

func (s *Server) Close(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}

	return s.server.Shutdown(ctx)
}

func normalizeLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}

	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
