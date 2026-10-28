package orders

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/payments"
)

func call(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-User", "alice")
	r.Header.Set("X-Forwarded-Client-Cert", "URI=spiffe://cluster.local/ns/apps/sa/frontend")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestCreateAndGet(t *testing.T) {
	pay := httptest.NewServer(payments.New())
	defer pay.Close()
	h := New(pay.URL+"/", pay.Client())

	rec := call(h, http.MethodPost, "/orders", `{"item":"book","amount_cents":1250}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var o Order
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	if o.OrderID != "ord-1" || o.User != "alice" || o.Payment.PaymentID != "pay-1" || o.Payment.OrderID != "ord-1" ||
		o.Caller != "spiffe://cluster.local/ns/apps/sa/frontend" {
		t.Errorf("order %+v", o)
	}

	rec = call(h, http.MethodGet, "/orders/ord-1", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"item":"book"`) {
		t.Errorf("get: %d %s", rec.Code, rec.Body)
	}
	if rec := call(h, http.MethodGet, "/orders/ord-404", ""); rec.Code != http.StatusNotFound {
		t.Errorf("get missing: %d", rec.Code)
	}
	if rec := call(h, http.MethodGet, "/healthz", ""); rec.Code != http.StatusOK {
		t.Errorf("healthz: %d", rec.Code)
	}
}

func TestCreateValidation(t *testing.T) {
	h := New("http://127.0.0.1:1", http.DefaultClient)
	for _, body := range []string{`{`, `{"item":"","amount_cents":1}`, `{"item":"x","amount_cents":0}`} {
		if rec := call(h, http.MethodPost, "/orders", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
}

func TestPaymentFailures(t *testing.T) {
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "RBAC: access denied", http.StatusForbidden)
	}))
	defer denied.Close()
	rec := call(New(denied.URL, denied.Client()), http.MethodPost, "/orders", `{"item":"x","amount_cents":1}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "RBAC: access denied") {
		t.Errorf("denied: %d %s", rec.Code, rec.Body)
	}

	rec = call(New("http://127.0.0.1:1", http.DefaultClient), http.MethodPost, "/orders", `{"item":"x","amount_cents":1}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("unreachable: %d", rec.Code)
	}
}
