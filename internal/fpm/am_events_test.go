package fpm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAMFusionProgressWhileVersionPending(t *testing.T) {
	output := "- APPNAME | VERSION | TYPE | SIZE\n◆ Firefox-AppImage | pending | appimage | 12.5MiB\n◆ Krita | 5.2.0 | appimage🔒 | 250MiB\n"
	packages := parseAMFusionProgress(output)
	if len(packages) != 2 {
		t.Fatalf("expected two AM progress inventory records, got %+v", packages)
	}
	if packages[0].Version != "pending" || packages[0].SizeBytes != 13107200 {
		t.Fatalf("pending-version progress did not preserve parsed downloaded bytes: %+v", packages[0])
	}
}

func TestAMDownloadEventsUseStableJSONPrefixAndDeduplicateProgress(t *testing.T) {
	var output strings.Builder
	last := make(map[string]int64)
	inventory := "◆ Firefox-AppImage | pending | appimage | 12.5MiB\n"
	emitAMDownloadEvents(&output, inventory, []string{"firefox"}, last)
	emitAMDownloadEvents(&output, inventory, []string{"firefox"}, last)
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], aimgEventPrefix) {
		t.Fatalf("expected one prefixed progress event, got %q", output.String())
	}
	var event aimgEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[0], aimgEventPrefix)), &event); err != nil {
		t.Fatalf("invalid event JSON: %v", err)
	}
	if event.Protocol != "fpm-aimg-v1" || event.Event != "download" || event.Name != "Firefox-AppImage" || event.Version != "pending" || event.DownloadedBytes != 13107200 {
		t.Fatalf("unexpected progress event: %+v", event)
	}
}

func TestAMInstalledEventReportsActualInventoryFields(t *testing.T) {
	var output strings.Builder
	emitAMInstalledEvents(&output, map[string]amFusionPackage{
		"Krita": {Name: "Krita", Version: "5.2.0", SizeBytes: 262144000, Installed: true},
	}, []string{"Krita"})
	line := strings.TrimSpace(output.String())
	if !strings.HasPrefix(line, aimgEventPrefix) {
		t.Fatalf("missing event prefix: %q", line)
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, aimgEventPrefix)), &event); err != nil {
		t.Fatal(err)
	}
	if event["protocol"] != "fpm-aimg-v1" || event["event"] != "installed" || event["name"] != "Krita" || event["version"] != "5.2.0" || event["size_bytes"] != float64(262144000) {
		t.Fatalf("unexpected installed event: %+v", event)
	}
}
