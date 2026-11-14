package fpm

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFakeBackend(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func isolateAMBackendTestState(t *testing.T) {
	t.Helper()
	previousPath := amFusionDBPathOverride
	amFusionDBPathOverride = filepath.Join(t.TempDir(), "am-fusion.db")
	t.Cleanup(func() { amFusionDBPathOverride = previousPath })
	t.Setenv("XDG_BIN_HOME", filepath.Join(t.TempDir(), "xdg-bin"))
}

func TestResolveAMBackendPrefersAppManForUserOperations(t *testing.T) {
	isolateAMBackendTestState(t)
	directory := t.TempDir()
	_ = writeFakeBackend(t, directory, "am")
	appmanPath := writeFakeBackend(t, directory, "appman")
	t.Setenv("PATH", directory)

	got, err := resolveAMBackend()
	if err != nil {
		t.Fatal(err)
	}
	if got != appmanPath {
		t.Fatalf("resolveAMBackend() = %q, want user-level AppMan %q", got, appmanPath)
	}
}

func TestResolveAMBackendHonorsStoredFrontendPreference(t *testing.T) {
	isolateAMBackendTestState(t)
	directory := t.TempDir()
	amPath := writeFakeBackend(t, directory, "am")
	appmanPath := writeFakeBackend(t, directory, "appman")
	t.Setenv("PATH", directory)
	if err := setAMFusionBackend("am"); err != nil {
		t.Fatal(err)
	}
	got, err := resolveAMBackend()
	if err != nil || got != amPath {
		t.Fatalf("stored AM preference resolved to %q, err=%v; want %q", got, err, amPath)
	}
	if err := setAMFusionBackend("appman"); err != nil {
		t.Fatal(err)
	}
	got, err = resolveAMBackend()
	if err != nil || got != appmanPath {
		t.Fatalf("stored AppMan preference resolved to %q, err=%v; want %q", got, err, appmanPath)
	}
}

func TestResolveAMBackendFallsBackToAppMan(t *testing.T) {
	isolateAMBackendTestState(t)
	directory := t.TempDir()
	appmanPath := writeFakeBackend(t, directory, "appman")
	t.Setenv("PATH", directory)

	got, err := resolveAMBackend()
	if err != nil {
		t.Fatal(err)
	}
	if got != appmanPath {
		t.Fatalf("resolveAMBackend() = %q, want %q", got, appmanPath)
	}
}

func TestResolveAMBackendReportsMissingExecutable(t *testing.T) {
	isolateAMBackendTestState(t)
	t.Setenv("PATH", t.TempDir())
	if _, err := resolveAMBackend(); err == nil {
		t.Fatal("resolveAMBackend() should fail when neither backend is installed")
	}
}
