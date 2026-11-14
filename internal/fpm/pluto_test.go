package fpm

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestSplitPlutoPackageVersion(t *testing.T) {
	cases := []struct {
		input, wantName, wantVersion string
	}{
		{"firefox-128.0_1", "firefox", "128.0_1"},
		{"firefox-i18n-128.0_1", "firefox-i18n", "128.0_1"},
		{"0ad-0.27.1_2", "0ad", "0.27.1_2"},
		{"plain-name", "plain-name", ""},
	}
	for _, tc := range cases {
		name, version := splitPlutoPackageVersion(tc.input)
		if name != tc.wantName || version != tc.wantVersion {
			t.Errorf("splitPlutoPackageVersion(%q) = (%q, %q), want (%q, %q)",
				tc.input, name, version, tc.wantName, tc.wantVersion)
		}
	}
}

func TestPlutoInstalledPackage(t *testing.T) {
	listing := "ii package-a-1.2_1 description\nii firefox-128.0_1 browser\n"
	if !plutoInstalledPackage("firefox", listing) {
		t.Fatal("expected firefox to be recognized as installed")
	}
	if plutoInstalledPackage("not-installed", listing) {
		t.Fatal("unexpected package match")
	}
}

func TestPlutoVersionAndQueryLimits(t *testing.T) {
	if plutoVersion != 1 || plutoVersion2 != 2 || plutoMaxQueryLength != 512 {
		t.Fatalf("unexpected PLUTO limits/version: v1=%d v2=%d query=%d", plutoVersion, plutoVersion2, plutoMaxQueryLength)
	}
}

func TestPlutoV2HelloAdvertisesMirrors(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStdout := os.Stdout
	os.Stdout = writer
	code := runPluto([]string{"v2", "hello"})
	_ = writer.Close()
	os.Stdout = previousStdout
	if code != 0 {
		t.Fatalf("runPluto hello returned %d", code)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	var message struct {
		Payload struct {
			Capabilities []string `json:"capabilities"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatalf("invalid PLUTO hello response: %s (%v)", data, err)
	}
	found := map[string]bool{}
	for _, capability := range message.Payload.Capabilities {
		found[capability] = true
	}
	for _, capability := range []string{"mirrors", "search-am", "kernels"} {
		if !found[capability] {
			t.Fatalf("PLUTO v2 hello did not advertise %s: %s", capability, data)
		}
	}
}

func TestPlutoPackageNameValidation(t *testing.T) {
	for _, name := range []string{"firefox", "0ad", "libfoo++", "python3.12", "foo_bar"} {
		if !plutoPackageName.MatchString(name) {
			t.Errorf("expected package name %q to be accepted", name)
		}
	}
	for _, name := range []string{"", "-R", "../tmp", "foo bar", "foo;rm"} {
		if plutoPackageName.MatchString(name) {
			t.Errorf("expected package name %q to be rejected", name)
		}
	}
}

func TestYelenaAppletCommandValidation(t *testing.T) {
	for _, command := range []string{"/usr/bin/python3\x00/home/user/yel-soft/applet.py\x00", "yl-soft-applet\x00--background-only"} {
		if !isYelenaAppletCommand(command) {
			t.Errorf("expected applet command %q to be recognized", command)
		}
	}
	for _, command := range []string{"/usr/bin/other-app\x00", "python3\x00not-applet.py\x00"} {
		if isYelenaAppletCommand(command) {
			t.Errorf("unexpected applet match for %q", command)
		}
	}
}
