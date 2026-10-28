package reports

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReports(t *testing.T) {
	h := New()
	r := httptest.NewRequest(http.MethodGet, "/reports", nil)
	r.Header.Set("X-User", "bob")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var rep Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Service != "reports" || rep.User != "bob" || len(rep.Rows) == 0 {
		t.Errorf("report %+v", rep)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/reports", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /reports: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthz: %d", rec.Code)
	}
}
