package frontend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/orders"
	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/payments"
)

const gatewayID = "spiffe://cluster.local/ns/istio-system/sa/istio-ingressgateway-service-account"

func req(h http.Handler, method, path, user, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		r.Header.Set("X-User", user)
	}
	r.Header.Set("X-Forwarded-Client-Cert", "URI="+gatewayID)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestCheckoutFullChain(t *testing.T) {
	pay := httptest.NewServer(payments.New())
	defer pay.Close()
	ord := httptest.NewServer(orders.New(pay.URL, pay.Client()))
	defer ord.Close()
	h := New(ord.URL, ord.Client())

	rec := req(h, http.MethodPost, "/checkout", "alice", `{"item":"book","amount_cents":1250}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("checkout: %d %s", rec.Code, rec.Body)
	}
	var resp CheckoutResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.User != "alice" || resp.Caller != gatewayID || resp.Order.User != "alice" ||
		resp.Order.Payment.Status != "captured" || resp.Order.Payment.AmountCents != 1250 {
		t.Errorf("response %+v", resp)
	}
}

func TestCheckoutRequiresUser(t *testing.T) {
	h := New("http://127.0.0.1:1", http.DefaultClient)
	if rec := req(h, http.MethodPost, "/checkout", "", `{}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("no user: %d", rec.Code)
	}
	if rec := req(h, http.MethodPost, "/checkout", "alice", `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", rec.Code)
	}
}

func TestCheckoutUpstreamErrors(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"item and a positive amount_cents are required"}`, http.StatusBadRequest)
	}))
	defer bad.Close()
	if rec := req(New(bad.URL, bad.Client()), http.MethodPost, "/checkout", "alice", `{"item":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("4xx passthrough: %d", rec.Code)
	}
	if rec := req(New("http://127.0.0.1:1", http.DefaultClient), http.MethodPost, "/checkout", "alice", `{"item":"x","amount_cents":1}`); rec.Code != http.StatusBadGateway {
		t.Errorf("unreachable: %d", rec.Code)
	}
}

func TestIndexAndHealth(t *testing.T) {
	h := New("http://unused", http.DefaultClient)
	rec := req(h, http.MethodGet, "/", "bob", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"user":"bob"`) {
		t.Errorf("index: %d %s", rec.Code, rec.Body)
	}
	if rec := req(h, http.MethodGet, "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Errorf("healthz: %d", rec.Code)
	}
	if rec := req(h, http.MethodGet, "/nope", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown path: %d", rec.Code)
	}
}
