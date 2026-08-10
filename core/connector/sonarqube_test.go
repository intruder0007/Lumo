// core/connector/sonarqube_test.go
package connector

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/intruder0007/Lumo/core/diag"
)

func TestSonarQubeConnectorReportsConnectedOnValidStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/system/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got == "" {
			t.Errorf("Authorization header not set")
		}
		w.Write([]byte(`{"status":"UP"}`))
	}))
	defer srv.Close()

	c := NewSonarQubeConnector(srv.URL, "test-token", srv.Client())
	res, err := RunEngine(c, diag.NoopLogger{})
	if err != nil {
		t.Fatalf("RunEngine: %v", err)
	}
	if !res.Connected {
		t.Errorf("Connected = false, want true")
	}
}

func TestSonarQubeConnectorFailsAtAuthOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewSonarQubeConnector(srv.URL, "bad-token", srv.Client())
	_, err := RunEngine(c, diag.NoopLogger{})
	if err == nil {
		t.Fatal("RunEngine: want error on 401, got nil")
	}
	perr, ok := err.(*PhaseError)
	if !ok {
		t.Fatalf("error type = %T, want *PhaseError", err)
	}
	if perr.Phase != "auth" {
		t.Errorf("PhaseError.Phase = %q, want %q", perr.Phase, "auth")
	}
}
