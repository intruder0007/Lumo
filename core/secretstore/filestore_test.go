package secretstore

import (
	"runtime"
	"testing"
)

func withTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	withTempConfigDir(t)
	fs, err := newFileStore()
	if err != nil {
		t.Fatalf("newFileStore: %v", err)
	}

	if _, found, err := fs.Get("sonarqube-token"); err != nil || found {
		t.Fatalf("Get on empty store: found=%v err=%v, want found=false err=nil", found, err)
	}

	if err := fs.Set("sonarqube-token", "abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, found, err := fs.Get("sonarqube-token")
	if err != nil || !found || got != "abc123" {
		t.Fatalf("Get after Set: got=%q found=%v err=%v, want got=%q found=true err=nil", got, found, err, "abc123")
	}

	if err := fs.Delete("sonarqube-token"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, err := fs.Get("sonarqube-token"); err != nil || found {
		t.Fatalf("Get after Delete: found=%v err=%v, want found=false err=nil", found, err)
	}
}

func TestFileStorePermissionsAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on Windows")
	}
	withTempConfigDir(t)
	fs, err := newFileStore()
	if err != nil {
		t.Fatalf("newFileStore: %v", err)
	}
	if err := fs.Set("k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	info, err := osStat(fs.path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("secrets file mode = %o, want 0600", perm)
	}
}
