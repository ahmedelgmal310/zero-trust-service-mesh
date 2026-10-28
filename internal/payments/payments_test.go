package payments

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const ordersID = "spiffe://cluster.local/ns/apps/sa/orders"

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Forwarded-Client-Cert", "Hash=x;URI="+ordersID)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestCharge(t *testing.T) {
	h := New()
	rec := do(t, h, http.MethodPost, "/charge", `{"order_id":"ord-1","amount_cents":1999}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var p Payment
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.PaymentID != "pay-1" || p.OrderID != "ord-1" || p.AmountCents != 1999 || p.Status != "captured" || p.Caller != ordersID {
		t.Errorf("payment %+v", p)
	}
}

func TestChargeValidation(t *testing.T) {
	h := New()
	for _, body := range []string{`not json`, `{"order_id":"","amount_cents":5}`, `{"order_id":"o","amount_cents":0}`, `{"order_id":"o","amount_cents":-3}`} {
		if rec := do(t, h, http.MethodPost, "/charge", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", body, rec.Code)
		}
	}
}

func TestRoutes(t *testing.T) {
	h := New()
	do(t, h, http.MethodPost, "/charge", `{"order_id":"ord-1","amount_cents":1}`)
	rec := do(t, h, http.MethodGet, "/admin/refunds", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"captured_total":1`) {
		t.Errorf("refunds: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/healthz", ""); rec.Code != http.StatusOK {
		t.Errorf("healthz: %d", rec.Code)
	}
	// The app itself only rejects unknown methods with 405; the mesh is what
	// turns DELETE /charge into a 403 before it gets here.
	if rec := do(t, h, http.MethodDelete, "/charge", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /charge: %d", rec.Code)
	}
}

func TestChargeConcurrentIDsAreUnique(t *testing.T) {
	h := New()
	var wg sync.WaitGroup
	ids := make(chan string, 50)
	for range 50 {
		wg.Go(func() {
			rec := do(t, h, http.MethodPost, "/charge", `{"order_id":"o","amount_cents":1}`)
			var p Payment
			_ = json.Unmarshal(rec.Body.Bytes(), &p)
			ids <- p.PaymentID
		})
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate payment id %s", id)
		}
		seen[id] = true
	}
}
