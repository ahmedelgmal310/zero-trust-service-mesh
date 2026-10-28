// Package orders implements the orders service. It is called by frontend and
// is the only workload allowed to call payments.
package orders

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/payments"
	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/platform"
)

// OrderRequest is the body of POST /orders.
type OrderRequest struct {
	Item        string `json:"item"`
	AmountCents int64  `json:"amount_cents"`
}

// Order is a created order together with its payment.
type Order struct {
	Service     string           `json:"service"`
	OrderID     string           `json:"order_id"`
	User        string           `json:"user"`
	Item        string           `json:"item"`
	AmountCents int64            `json:"amount_cents"`
	Caller      string           `json:"caller"`
	Payment     payments.Payment `json:"payment"`
}

type service struct {
	paymentsURL string
	client      *http.Client

	mu     sync.Mutex
	orders map[string]Order
	seq    int
}

// New returns the orders HTTP handler. paymentsURL is the base URL of payments.
func New(paymentsURL string, client *http.Client) http.Handler {
	s := &service{
		paymentsURL: strings.TrimRight(paymentsURL, "/"),
		client:      client,
		orders:      map[string]Order{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", platform.Healthz)
	mux.HandleFunc("POST /orders", s.create)
	mux.HandleFunc("GET /orders/{id}", s.get)
	return mux
}

func (s *service) create(w http.ResponseWriter, r *http.Request) {
	var req OrderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		platform.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Item == "" || req.AmountCents <= 0 {
		platform.Error(w, http.StatusBadRequest, "item and a positive amount_cents are required")
		return
	}
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("ord-%d", s.seq)
	s.mu.Unlock()

	var p payments.Payment
	err := platform.PostJSON(r.Context(), s.client, s.paymentsURL+"/charge", nil,
		payments.ChargeRequest{OrderID: id, AmountCents: req.AmountCents}, &p)
	if err != nil {
		status := http.StatusBadGateway
		var ue *platform.UpstreamError
		if errors.As(err, &ue) && ue.Status == http.StatusForbidden {
			status = http.StatusForbidden
		}
		platform.Error(w, status, "payment failed: "+err.Error())
		return
	}
	o := Order{
		Service:     "orders",
		OrderID:     id,
		User:        r.Header.Get("X-User"),
		Item:        req.Item,
		AmountCents: req.AmountCents,
		Caller:      platform.Caller(r),
		Payment:     p,
	}
	s.mu.Lock()
	s.orders[id] = o
	s.mu.Unlock()
	platform.WriteJSON(w, http.StatusCreated, o)
}

func (s *service) get(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	o, ok := s.orders[r.PathValue("id")]
	s.mu.Unlock()
	if !ok {
		platform.Error(w, http.StatusNotFound, "order not found")
		return
	}
	platform.WriteJSON(w, http.StatusOK, o)
}
