// Package platform holds the plumbing shared by every service: configuration
// from the environment, JSON responses, request logging, mesh identity
// extraction and graceful shutdown.
package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Env returns the value of the environment variable key, or def when unset or empty.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// WriteJSON writes v as a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

// Error writes a JSON error body.
func Error(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// Healthz is the liveness/readiness handler used by every service.
func Healthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Caller returns the SPIFFE ID of the workload that called us, as asserted by
// the local sidecar in the X-Forwarded-Client-Cert header. Istio appends its own
// element to the header, so only the last element is trusted: anything before
// it was supplied by the client and may be forged. Returns "" when the request
// did not arrive over mesh mTLS.
func Caller(r *http.Request) string {
	xfcc := r.Header.Get("X-Forwarded-Client-Cert")
	if xfcc == "" {
		return ""
	}
	elems := strings.Split(xfcc, ",")
	last := elems[len(elems)-1]
	for _, kv := range strings.Split(last, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if ok && strings.EqualFold(k, "URI") {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Logged wraps h with structured access logging that records the mesh caller.
func Logged(service string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		if r.URL.Path == "/healthz" {
			return
		}
		slog.Info("request",
			"service", service,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"caller", Caller(r),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// NewServer returns an http.Server with conservative timeouts.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// Serve runs srv on ln until ctx is cancelled, then shuts down gracefully.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener) error {
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
