package fpm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyAMInstallerRejectsUnverifiedPayload(t *testing.T) {
	if err := verifyAMInstallerPayload([]byte("not the official installer")); err == nil {
		t.Fatal("expected checksum verification to reject an untrusted payload")
	}
	if err := verifyAMInstallerPayload(make([]byte, amInstallerMaxSize+1)); err == nil {
		t.Fatal("expected installer size limit to reject an oversized payload")
	}
}

func TestResolveAMBackendFindsLocalAppManOutsidePath(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	appman := filepath.Join(binDir, "appman")
	if err := os.WriteFile(appman, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_BIN_HOME", "")
	t.Setenv("PATH", t.TempDir())

	got, err := resolveAMBackend()
	if err != nil {
		t.Fatal(err)
	}
	if got != appman {
		t.Fatalf("resolveAMBackend() = %q, want %q", got, appman)
	}
}
