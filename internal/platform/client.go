package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// NewClient returns the HTTP client services use for downstream calls. The
// sidecar upgrades these plaintext calls to mTLS transparently.
func NewClient() *http.Client {
	return &http.Client{Timeout: 3 * time.Second}
}

// UpstreamError reports a non-2xx answer from a downstream service.
type UpstreamError struct {
	URL    string
	Status int
	Body   string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream %s returned %d: %s", e.URL, e.Status, e.Body)
}

// PostJSON sends in as JSON to url and decodes the JSON answer into out.
// Extra headers (for example the end-user identity) are forwarded as given.
func PostJSON(ctx context.Context, c *http.Client, url string, headers map[string]string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read %s: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &UpstreamError{URL: url, Status: resp.StatusCode, Body: string(bytes.TrimSpace(raw))}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

// Run is the shared main: JSON logs, listen on $ADDR (default :8080), serve
// until SIGINT/SIGTERM. It returns the process exit code.
func Run(service string, h http.Handler) int {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, service, Env("ADDR", ":8080"), h)
}

func run(ctx context.Context, service, addr string, h http.Handler) int {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		slog.Error("listen", "service", service, "addr", addr, "err", err)
		return 1
	}
	slog.Info("listening", "service", service, "addr", ln.Addr().String())
	if err := Serve(ctx, NewServer(addr, Logged(service, h)), ln); err != nil {
		slog.Error("server stopped", "service", service, "err", err)
		return 1
	}
	slog.Info("stopped", "service", service)
	return 0
}
