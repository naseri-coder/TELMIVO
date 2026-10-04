package httpapi

import (
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		e      error
		status int
	}{{errors.New("database connection failed"), 500}, {database.ErrInvalid, 400}, {database.ErrForbidden, 403}, {database.ErrConflict, 409}} {
		w := httptest.NewRecorder()
		handleError(w, tc.e)
		if w.Code != tc.status {
			t.Fatalf("got %d expected %d", w.Code, tc.status)
		}
	}
}
func TestOriginTrust(t *testing.T) {
	a := API{Config: config.Config{MiniAppURL: "https://mini.example"}}
	handler := a.secure(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, tc := range []struct {
		origin string
		status int
	}{{"https://mini.example", 204}, {"http://mini.example", 403}, {"https://mini.example.evil", 403}, {"https://api.example", 403}, {"null", 403}, {"https://mini.example/path", 403}} {
		r := httptest.NewRequest("POST", "https://api.example/api/test", nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.origin, w.Code)
		}
	}
}

func TestReadinessFailsClosedWithoutDependencies(t *testing.T) {
	a := API{}
	w := httptest.NewRecorder()
	a.ready(w, httptest.NewRequest("GET", "/health/ready", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
