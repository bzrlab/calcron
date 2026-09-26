package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/calcron/calcron/internal/server"
)

// The embedded dashboard must be served at / with a real HTML document and
// hashed assets. A missing build leaves a placeholder page, which still parses
// but has no script tag — this asserts the built app is embedded.
func TestDashboardServesEmbeddedBuild(t *testing.T) {
	s := &server.Server{}
	h := httptest.NewServer(s.Handler())
	defer h.Close()

	res, err := http.Get(h.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	html := string(body)
	if !strings.Contains(html, `<div id="root">`) {
		t.Fatalf("dashboard missing app root: %s", html)
	}
	if !strings.Contains(html, "/assets/") {
		t.Fatalf("dashboard missing built asset references: %s", html)
	}
	if strings.Contains(html, "not built") {
		t.Fatal("dashboard served the placeholder page — run `cd web && npm run build`")
	}
}

func TestHealth(t *testing.T) {
	s := &server.Server{}
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	res, err := http.Get(h.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("GET /health status = %d", res.StatusCode)
	}
}
