// Package frontend implements the edge-facing service. It is reached only
// through the ingress gateway, after the gateway has validated the end-user
// JWT, and it calls orders on the user's behalf.
package frontend

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/orders"
	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/platform"
)

// CheckoutRequest is the body of POST /checkout.
type CheckoutRequest struct {
	Item        string `json:"item"`
	AmountCents int64  `json:"amount_cents"`
}

// CheckoutResponse shows the whole call chain, including the mesh identity
// each hop saw for its caller.
type CheckoutResponse struct {
	Service string       `json:"service"`
	User    string       `json:"user"`
	Caller  string       `json:"caller"`
	Order   orders.Order `json:"order"`
}

type service struct {
	ordersURL string
	client    *http.Client
}

// New returns the frontend HTTP handler. ordersURL is the base URL of orders.
func New(ordersURL string, client *http.Client) http.Handler {
	s := &service{ordersURL: strings.TrimRight(ordersURL, "/"), client: client}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", platform.Healthz)
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("POST /checkout", s.checkout)
	return mux
}

func (s *service) index(w http.ResponseWriter, r *http.Request) {
	platform.WriteJSON(w, http.StatusOK, map[string]string{
		"service": "frontend",
		"user":    r.Header.Get("X-User"),
		"caller":  platform.Caller(r),
	})
}

func (s *service) checkout(w http.ResponseWriter, r *http.Request) {
	// X-User is written by the gateway from the verified JWT "sub" claim.
	user := r.Header.Get("X-User")
	if user == "" {
		platform.Error(w, http.StatusUnauthorized, "missing end-user identity")
		return
	}
	var req CheckoutRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		platform.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var o orders.Order
	err := platform.PostJSON(r.Context(), s.client, s.ordersURL+"/orders",
		map[string]string{"X-User": user},
		orders.OrderRequest(req), &o)
	if err != nil {
		status := http.StatusBadGateway
		var ue *platform.UpstreamError
		if errors.As(err, &ue) && ue.Status < 500 {
			status = ue.Status
		}
		platform.Error(w, status, "order failed: "+err.Error())
		return
	}
	platform.WriteJSON(w, http.StatusCreated, CheckoutResponse{
		Service: "frontend",
		User:    user,
		Caller:  platform.Caller(r),
		Order:   o,
	})
}
