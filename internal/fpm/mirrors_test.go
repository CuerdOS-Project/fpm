package fpm

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseXBPSRepositoryURLs(t *testing.T) {
	got := parseXBPSRepositoryURLs("" +
		"  1200 https://repo-de.voidlinux.org/current (RSA signed)\n" +
		"  -1 https://repo-de.voidlinux.org/current (unavailable)\n" +
		"  900 https://pkgs.cuerdos.es/stable\n")
	want := []string{
		"https://repo-de.voidlinux.org/current",
		"https://pkgs.cuerdos.es/stable",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseXBPSRepositoryURLs() = %#v, want %#v", got, want)
	}
}

func TestVoidFallbackURLForOfficialRepositories(t *testing.T) {
	cases := map[string]string{
		"https://repo-de.voidlinux.org/current":                      "https://repo-default.voidlinux.org/current",
		"https://mirrors.summithq.com/voidlinux/current/nonfree":     "https://repo-default.voidlinux.org/current/nonfree",
		"http://ftp.dk.xemacs.org/voidlinux/current/multilib":        "https://repo-default.voidlinux.org/current/multilib",
		"https://repo-fastly.voidlinux.org/current/multilib/nonfree": "https://repo-default.voidlinux.org/current/multilib/nonfree",
	}
	for input, want := range cases {
		if got, ok := voidFallbackURL(input); !ok || got != want {
			t.Errorf("voidFallbackURL(%q) = (%q, %t), want (%q, true)", input, got, ok, want)
		}
	}
	for _, input := range []string{
		"https://repo-default.voidlinux.org/current",
		"https://pkgs.cuerdos.es/current",
		"https://repo-de.voidlinux.org/stable",
		"https://repo-de.voidlinux.org/current/unknown",
		"file:///current",
		"https://repo-de.voidlinux.org:8443/current",
	} {
		if got, ok := voidFallbackURL(input); ok {
			t.Errorf("voidFallbackURL(%q) unexpectedly returned %q", input, got)
		}
	}
}

func TestFallbackConfigWritesOnlyGlobalURLsAndIsReversible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xbps.d", "zzzz-fpm-void-fallback.conf")
	urls := []string{
		"https://repo-default.voidlinux.org/current/nonfree",
		"https://repo-default.voidlinux.org/current",
		"https://repo-default.voidlinux.org/current",
	}
	if err := writeVoidFallbackConfig(path, urls); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, voidFallbackMarker+"\n") ||
		!strings.Contains(text, "repository=https://repo-default.voidlinux.org/current\n") ||
		!strings.Contains(text, "repository=https://repo-default.voidlinux.org/current/nonfree\n") {
		t.Fatalf("unexpected fallback config: %s", text)
	}
	if strings.Count(text, "repository=") != 2 {
		t.Fatalf("expected duplicate URLs to be removed, got: %s", text)
	}
	managed, readURLs, err := readVoidFallbackConfig(path)
	if err != nil || !managed || len(readURLs) != 2 {
		t.Fatalf("readVoidFallbackConfig() = (%t, %#v, %v)", managed, readURLs, err)
	}
	removed, err := removeVoidFallbackConfig(path)
	if err != nil || !removed {
		t.Fatalf("removeVoidFallbackConfig() = (%t, %v)", removed, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed fallback file still exists: %v", err)
	}
}

func TestFallbackConfigRefusesUnmanagedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.conf")
	if err := os.WriteFile(path, []byte("repository=https://example.test/repo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeVoidFallbackConfig(path, []string{"https://repo-default.voidlinux.org/current"}); !errors.Is(err, errVoidFallbackUnmanaged) {
		t.Fatalf("writeVoidFallbackConfig error = %v, want unmanaged-file error", err)
	}
	if _, err := removeVoidFallbackConfig(path); !errors.Is(err, errVoidFallbackUnmanaged) {
		t.Fatalf("removeVoidFallbackConfig error = %v, want unmanaged-file error", err)
	}
}

func TestFallbackConfigRejectsNonGlobalOrNonVoidURLs(t *testing.T) {
	for _, urls := range [][]string{
		{"https://pkgs.cuerdos.es/current"},
		{"https://repo-de.voidlinux.org/current"},
		{"https://repo-default.voidlinux.org/current/evil"},
	} {
		if _, err := fallbackConfigContents(urls); err == nil {
			t.Errorf("fallbackConfigContents(%#v) unexpectedly succeeded", urls)
		}
	}
}

func TestMirrorPlutoRecordsPreserveFallbackState(t *testing.T) {
	records := mirrorPlutoRecords(
		[]string{
			"https://repo-de.voidlinux.org/current",
			"https://repo-default.voidlinux.org/current",
			"https://pkgs.cuerdos.es/stable",
		},
		[]string{"https://repo-default.voidlinux.org/current"},
	)
	if len(records) != 2 {
		t.Fatalf("got %d records, want only the two official Void mirrors: %#v", len(records), records)
	}
	if records[0]["fallback_url"] != "https://repo-default.voidlinux.org/current" || records[0]["fallback_configured"] != true {
		t.Fatalf("primary mirror fallback metadata missing: %#v", records[0])
	}
	if records[1]["is_fallback"] != true || records[1]["official_void"] != true {
		t.Fatalf("fallback row not identified: %#v", records[1])
	}
}

func TestPlutoMirrorsEmitsCompleteV2Stream(t *testing.T) {
	dir := t.TempDir()
	query := filepath.Join(dir, "xbps-query")
	script := "#!/bin/sh\nprintf '%s\\n' '1200 https://repo-de.voidlinux.org/current (RSA signed)'\n"
	if err := os.WriteFile(query, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	previousPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+previousPath); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Setenv("PATH", previousPath) }()
	previousConfigPath := voidFallbackConfigPath
	voidFallbackConfigPath = filepath.Join(dir, "xbps.d", "zzzz-fpm-void-fallback.conf")
	defer func() { voidFallbackConfigPath = previousConfigPath }()
	if err := writeVoidFallbackConfig(voidFallbackConfigPath, []string{"https://repo-default.voidlinux.org/current"}); err != nil {
		t.Fatal(err)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStdout := os.Stdout
	os.Stdout = writer
	code := plutoMirrors(nil)
	_ = writer.Close()
	os.Stdout = previousStdout
	if code != 0 {
		t.Fatalf("plutoMirrors returned %d", code)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	var messages []struct {
		Signal  string         `json:"signal"`
		Payload map[string]any `json:"payload"`
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var message struct {
			Signal  string         `json:"signal"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("invalid PLUTO JSON line %q: %v", line, err)
		}
		messages = append(messages, message)
	}
	if len(messages) != 2 || messages[0].Signal != "mirrors.result" || messages[1].Signal != "mirrors.done" {
		t.Fatalf("unexpected PLUTO mirror stream: %s", data)
	}
	if messages[0].Payload["fallback_configured"] != true || messages[1].Payload["fallback_enabled"] != true {
		t.Fatalf("fallback state missing from PLUTO response: %s", data)
	}
}
