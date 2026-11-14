package fpm

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupSearchSourceBackends(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	writeSearchTestCommand(t, binDir, "xbps-query", `#!/bin/sh
case "$1" in
  -Rs) printf '[*] firefox-128.0_1 XBPS web browser\n' ;;
  -l) printf '[*] firefox-128.0_1 XBPS web browser\n' ;;
  *) exit 2 ;;
esac
`)
	writeSearchTestCommand(t, binDir, "xbps-install", "#!/bin/sh\nexit 0\n")
	writeSearchTestCommand(t, binDir, "am", `#!/bin/sh
if [ "$1" = "-q" ] && [ "$2" = "--appimages" ]; then
  printf '◆ Firefox-AppImage : AM graphical browser\n◆ Krita-AppImage : AM digital painting\n'
  exit 0
fi
exit 2
`)
	t.Setenv("PATH", binDir)
	oldDBPath := amFusionDBPathOverride
	amFusionDBPathOverride = filepath.Join(t.TempDir(), "am-fusion.db")
	t.Cleanup(func() { amFusionDBPathOverride = oldDBPath })
	catalog := []amFusionPackage{
		{Name: "Firefox-AppImage", Description: "AM graphical browser", Type: "appimage"},
		{Name: "Krita-AppImage", Description: "AM digital painting", Type: "appimage"},
	}
	installed := map[string]amFusionPackage{
		"Firefox-AppImage": {Name: "Firefox-AppImage", Version: "128.0", Type: "appimage", SizeBytes: 94371840, Installed: true},
	}
	if err := saveAMFusionSnapshot(catalog, installed); err != nil {
		t.Fatal(err)
	}
	if err := setAMFusionEnabled(true); err != nil {
		t.Fatal(err)
	}
}

func writeSearchTestCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
}

func captureSearchStdout(t *testing.T, call func() int) (int, string) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	code := call()
	os.Stdout = previous
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return code, string(data)
}

func TestSearchAllIncludesAMFusionOutsideTTY(t *testing.T) {
	setupSearchSourceBackends(t)
	_, output := captureSearchStdout(t, func() int {
		searchAll("browser")
		return 0
	})
	if !strings.Contains(output, "[am-FUSION]") || !strings.Contains(output, "Firefox-AppImage") || !strings.Contains(output, "AM graphical browser") {
		t.Fatalf("global search omitted AM results:\n%s", output)
	}
	if !strings.Contains(output, "XBPS") || !strings.Contains(output, "firefox") {
		t.Fatalf("global search omitted XBPS results:\n%s", output)
	}
}

func TestParseAMFusionSearchItemsIncludesCachedInstalledState(t *testing.T) {
	items := parseAMFusionSearchItems("◆ Firefox-AppImage : AM graphical browser\n", map[string]amFusionPackage{
		"firefox-appimage": {Name: "Firefox-AppImage", Version: "128.0", Installed: true},
	})
	if len(items) != 1 {
		t.Fatalf("expected one AM result, got %+v", items)
	}
	item := items[0]
	if item.Source != amFusionRepoName || !item.Installed || !item.Selectable || item.Name != "Firefox-AppImage" {
		t.Fatalf("unexpected AM search item: %+v", item)
	}
}

func TestPlutoV2SearchIncludesAMAndPreservesV1Shape(t *testing.T) {
	setupSearchSourceBackends(t)
	code, output := captureSearchStdout(t, func() int {
		return runPluto([]string{"v2", "search", "browser"})
	})
	if code != 0 {
		t.Fatalf("PLUTO v2 search returned %d: %s", code, output)
	}
	var results []map[string]any
	var done map[string]any
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var message struct {
			Signal  string         `json:"signal"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			t.Fatalf("invalid JSON Lines response %q: %v", scanner.Text(), err)
		}
		if message.Signal == "search.result" {
			results = append(results, message.Payload)
		}
		if message.Signal == "search.done" {
			done = message.Payload
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	foundXBPS, foundAM := false, false
	for _, result := range results {
		switch result["manager"] {
		case "xbps":
			foundXBPS = foundXBPS || result["name"] == "firefox"
		case "am":
			foundAM = foundAM || (result["name"] == "Firefox-AppImage" && result["repository"] == amFusionRepoName && result["installed"] == true && result["size_bytes"] == float64(94371840) && result["icon_url"] == amFusionIconURL("Firefox-AppImage"))
		}
	}
	if !foundXBPS || !foundAM {
		t.Fatalf("PLUTO v2 did not return both attributed sources: %+v", results)
	}
	if done == nil || done["complete"] != true || done["count"] != float64(3) {
		t.Fatalf("unexpected PLUTO completion record: %+v", done)
	}

	code, output = captureSearchStdout(t, func() int {
		return runPluto([]string{"v1", "search", "browser"})
	})
	if code != 0 {
		t.Fatalf("PLUTO v1 search returned %d: %s", code, output)
	}
	if strings.Contains(output, "Firefox-AppImage") || strings.Contains(output, `"manager"`) {
		t.Fatalf("PLUTO v1 format/source behavior changed: %s", output)
	}
}

func TestPlutoV2SearchCanUseAMWithoutXBPS(t *testing.T) {
	binDir := t.TempDir()
	writeSearchTestCommand(t, binDir, "am", `#!/bin/sh
if [ "$1" = "-q" ]; then
  printf '◆ Krita-AppImage : Digital painting\n'
  exit 0
fi
exit 2
`)
	t.Setenv("PATH", binDir)
	oldDBPath := amFusionDBPathOverride
	amFusionDBPathOverride = filepath.Join(t.TempDir(), "am-fusion.db")
	t.Cleanup(func() { amFusionDBPathOverride = oldDBPath })
	if err := saveAMFusionSnapshot([]amFusionPackage{{Name: "Krita-AppImage", Description: "Digital painting", Type: "appimage"}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := setAMFusionEnabled(true); err != nil {
		t.Fatal(err)
	}
	code, output := captureSearchStdout(t, func() int {
		return runPluto([]string{"v2", "search", "krita"})
	})
	if code != 0 {
		t.Fatalf("AM-only PLUTO search returned %d: %s", code, output)
	}
	if !strings.Contains(output, `"signal":"search.source_error"`) || !strings.Contains(output, `"manager":"am"`) || !strings.Contains(output, `"name":"Krita-AppImage"`) {
		t.Fatalf("AM-only PLUTO search did not report a partial XBPS source and the AM result: %s", output)
	}
}
