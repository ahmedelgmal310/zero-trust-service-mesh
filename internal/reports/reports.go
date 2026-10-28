// Package reports implements an analytics service that is unrelated to the
// checkout flow. It has no legitimate reason to talk to payments, which makes
// it the "compromised neighbour" in the attack suite.
package reports

import (
	"net/http"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/platform"
)

// Report is the answer of GET /reports.
type Report struct {
	Service string   `json:"service"`
	Name    string   `json:"name"`
	Rows    []string `json:"rows"`
	User    string   `json:"user"`
	Caller  string   `json:"caller"`
}

// New returns the reports HTTP handler.
func New() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", platform.Healthz)
	mux.HandleFunc("GET /reports", func(w http.ResponseWriter, r *http.Request) {
		platform.WriteJSON(w, http.StatusOK, Report{
			Service: "reports",
			Name:    "daily-sales",
			Rows:    []string{"2026-10-27,412", "2026-10-28,389"},
			User:    r.Header.Get("X-User"),
			Caller:  platform.Caller(r),
		})
	})
	return mux
}
