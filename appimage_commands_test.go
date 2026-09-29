package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"My App 2.0": "my-app-20",
		"":           "app",
		"áccent!":    "ccent",
		"foo_bar":    "foo_bar",
	}
	for input, want := range cases {
		if got := slugify(input); got != want {
			t.Errorf("slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGuessName(t *testing.T) {
	if got := guessName("/tmp/My-App-2.3.AppImage"); got != "My App 2 3" {
		t.Fatalf("got %q", got)
	}
	if got := guessName("tool.appimage"); got != "tool" {
		t.Fatalf("got %q", got)
	}
}

func TestPackageInInstalledList(t *testing.T) {
	output := "[*] foo\n[-] bar\n"
	if !packageInInstalledList("foo", output) {
		t.Fatal("expected package to be found")
	}
	if packageInInstalledList("not-there", output) {
		t.Fatal("unexpected package match")
	}
}

func TestParseDesktop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.desktop")
	content := "[Desktop Entry]\nName=Demo\nExec=\"/opt/appimages/demo.AppImage\" %U\nIcon=/tmp/demo.png\nCategories=Utility;\nX-FPM-AppImage=true\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	app, ok := parseDesktop(path, "/opt/appimages")
	if !ok {
		t.Fatal("desktop file was not recognized")
	}
	if app.Name != "Demo" || app.AppImagePath != "/opt/appimages/demo.AppImage" || app.Category != "Utility;" {
		t.Fatalf("unexpected app: %+v", app)
	}
}

func TestParseSearchLines(t *testing.T) {
	items := parseSearchLines("[*] foo description here\n[-] bar another description\n", "XBPS", "[*] foo\n")
	if len(items) != 2 || items[0].Name != "foo" || !items[0].Installed {
		t.Fatalf("unexpected XBPS items: %+v", items)
	}
	flatpaks := parseSearchLines("GIMP\tImage editor\torg.gimp.GIMP\tflathub\n", "Flatpak", "org.gimp.GIMP\n")
	if len(flatpaks) != 1 || flatpaks[0].Name != "org.gimp.GIMP" || flatpaks[0].Label != "GIMP" || flatpaks[0].Remote != "flathub" || !flatpaks[0].Installed {
		t.Fatalf("unexpected Flatpak items: %+v", flatpaks)
	}
}

func TestXBPSCommandConfirmation(t *testing.T) {
	old := yesFlag
	defer func() { yesFlag = old }()
	yesFlag = false
	if got := xbpsCommand("xbps-install", "-S", "foo"); containsString(got, "-y") {
		t.Fatalf("unexpected automatic confirmation: %v", got)
	}
	yesFlag = true
	if got := xbpsCommand("xbps-install", "-S", "foo"); !containsString(got, "-y") {
		t.Fatalf("expected explicit confirmation flag: %v", got)
	}
}
