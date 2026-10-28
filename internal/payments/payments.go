// Package payments implements the payments service: the most sensitive
// workload in the demo. Only orders may call POST /charge, and nobody may
// reach /admin/refunds through the mesh.
package payments

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/platform"
)

// ChargeRequest is the body of POST /charge.
type ChargeRequest struct {
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
}

// Payment is the answer to a successful charge.
type Payment struct {
	Service     string `json:"service"`
	PaymentID   string `json:"payment_id"`
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Status      string `json:"status"`
	Caller      string `json:"caller"`
}

type service struct {
	mu       sync.Mutex
	payments []Payment
}

// New returns the payments HTTP handler.
func New() http.Handler {
	s := &service{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", platform.Healthz)
	mux.HandleFunc("POST /charge", s.charge)
	mux.HandleFunc("GET /admin/refunds", s.refunds)
	return mux
}

func (s *service) charge(w http.ResponseWriter, r *http.Request) {
	var req ChargeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		platform.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.OrderID == "" || req.AmountCents <= 0 {
		platform.Error(w, http.StatusBadRequest, "order_id and a positive amount_cents are required")
		return
	}
	s.mu.Lock()
	p := Payment{
		Service:     "payments",
		PaymentID:   fmt.Sprintf("pay-%d", len(s.payments)+1),
		OrderID:     req.OrderID,
		AmountCents: req.AmountCents,
		Status:      "captured",
		Caller:      platform.Caller(r),
	}
	s.payments = append(s.payments, p)
	s.mu.Unlock()
	platform.WriteJSON(w, http.StatusCreated, p)
}

// refunds is an internal admin endpoint. The application itself would happily
// answer; it is the AuthorizationPolicy that keeps every workload away from it.
func (s *service) refunds(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	n := len(s.payments)
	s.mu.Unlock()
	platform.WriteJSON(w, http.StatusOK, map[string]any{
		"service":         "payments",
		"captured_total":  n,
		"pending_refunds": []string{},
		"caller":          platform.Caller(r),
	})
}
