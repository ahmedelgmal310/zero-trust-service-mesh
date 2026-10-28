package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEnv(t *testing.T) {
	t.Setenv("ZT_TEST_SET", "value")
	t.Setenv("ZT_TEST_EMPTY", "")
	if got := Env("ZT_TEST_SET", "def"); got != "value" {
		t.Errorf("Env set = %q", got)
	}
	if got := Env("ZT_TEST_EMPTY", "def"); got != "def" {
		t.Errorf("Env empty = %q", got)
	}
	if got := Env("ZT_TEST_UNSET_XYZ", "def"); got != "def" {
		t.Errorf("Env unset = %q", got)
	}
}

func TestCaller(t *testing.T) {
	const orders = "spiffe://cluster.local/ns/apps/sa/orders"
	cases := []struct {
		name, xfcc, want string
	}{
		{"none", "", ""},
		{"istio", `By=spiffe://cluster.local/ns/apps/sa/payments;Hash=abc;Subject="";URI=` + orders, orders},
		{"quoted", `Hash=abc;URI="` + orders + `"`, orders},
		{"lowercase key", `hash=abc;uri=` + orders, orders},
		{"forged element is ignored", `URI=spiffe://cluster.local/ns/apps/sa/frontend,By=x;URI=` + orders, orders},
		{"no uri", `By=x;Hash=abc`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.xfcc != "" {
				r.Header.Set("X-Forwarded-Client-Cert", tc.xfcc)
			}
			if got := Caller(r); got != tc.want {
				t.Errorf("Caller = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWriteJSONAndHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	Healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("healthz: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("healthz body %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	Error(rec, http.StatusTeapot, "nope")
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] != "nope" || rec.Code != http.StatusTeapot {
		t.Errorf("Error: %d %v %v", rec.Code, body, err)
	}

	rec = httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, func() {}) // not encodable: logged, not panicking
	if rec.Code != http.StatusOK {
		t.Errorf("WriteJSON unencodable: %d", rec.Code)
	}
}

func TestLoggedRecordsStatus(t *testing.T) {
	h := Logged("test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	for _, p := range []string{"/x", "/healthz"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusAccepted {
			t.Errorf("%s: status %d", p, rec.Code)
		}
	}
}

func TestPostJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if r.Header.Get("X-User") != "alice" || r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, "missing headers", http.StatusBadRequest)
				return
			}
			var in map[string]int
			_ = json.NewDecoder(r.Body).Decode(&in)
			WriteJSON(w, http.StatusCreated, map[string]int{"double": in["n"] * 2})
		case "/denied":
			http.Error(w, "RBAC: access denied", http.StatusForbidden)
		case "/garbage":
			_, _ = w.Write([]byte("not json"))
		}
	}))
	defer srv.Close()
	c := srv.Client()
	ctx := context.Background()

	var out map[string]int
	if err := PostJSON(ctx, c, srv.URL+"/ok", map[string]string{"X-User": "alice"}, map[string]int{"n": 21}, &out); err != nil {
		t.Fatalf("ok: %v", err)
	}
	if out["double"] != 42 {
		t.Errorf("ok: got %v", out)
	}

	err := PostJSON(ctx, c, srv.URL+"/denied", nil, map[string]int{}, &out)
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.Status != http.StatusForbidden || ue.Body != "RBAC: access denied" {
		t.Fatalf("denied: %v", err)
	}
	if !strings.Contains(ue.Error(), "403") {
		t.Errorf("UpstreamError.Error = %q", ue.Error())
	}

	if err := PostJSON(ctx, c, srv.URL+"/garbage", nil, map[string]int{}, &out); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("garbage: %v", err)
	}
	if err := PostJSON(ctx, c, srv.URL+"/ok", nil, func() {}, &out); err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Errorf("marshal: %v", err)
	}
	if err := PostJSON(ctx, c, "http://[::1", nil, map[string]int{}, &out); err == nil || !strings.Contains(err.Error(), "build request") {
		t.Errorf("bad url: %v", err)
	}
	if err := PostJSON(ctx, c, "http://127.0.0.1:1/x", nil, map[string]int{}, &out); err == nil || !strings.Contains(err.Error(), "call") {
		t.Errorf("unreachable: %v", err)
	}
	if NewClient().Timeout <= 0 {
		t.Error("client must have a timeout")
	}
}

func TestServeGracefulShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(ln.Addr().String(), http.HandlerFunc(Healthz))
	if srv.ReadHeaderTimeout == 0 {
		t.Fatal("ReadHeaderTimeout must be set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, srv, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop")
	}
}

func TestServeListenerError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	if err := Serve(context.Background(), NewServer("", http.NotFoundHandler()), ln); err == nil {
		t.Fatal("expected error from closed listener")
	}
}

func TestRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	code := make(chan int, 1)
	go func() { code <- run(ctx, "test", "127.0.0.1:0", http.NotFoundHandler()) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	if c := <-code; c != 0 {
		t.Fatalf("run exit code %d", c)
	}
	if c := run(context.Background(), "test", "256.0.0.1:bad", http.NotFoundHandler()); c != 1 {
		t.Fatalf("bad addr exit code %d", c)
	}
}
