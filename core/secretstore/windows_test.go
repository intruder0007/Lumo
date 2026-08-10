//go:build windows

package secretstore

import "testing"

func TestWindowsStoreRoundTrip(t *testing.T) {
	withTempConfigDir(t)
	s, ok := nativeStore()
	if !ok {
		t.Fatal("nativeStore() on windows should always report ok=true")
	}

	if _, found, err := s.Get("sonarqube-token"); err != nil || found {
		t.Fatalf("Get on empty store: found=%v err=%v, want found=false err=nil", found, err)
	}

	if err := s.Set("sonarqube-token", "abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, found, err := s.Get("sonarqube-token")
	if err != nil || !found || got != "abc123" {
		t.Fatalf("Get after Set: got=%q found=%v err=%v, want got=%q found=true err=nil", got, found, err, "abc123")
	}

	if err := s.Delete("sonarqube-token"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, err := s.Get("sonarqube-token"); err != nil || found {
		t.Fatalf("Get after Delete: found=%v err=%v, want found=false err=nil", found, err)
	}
}
