package main

import "testing"

func TestSplitNissaPackageVersion(t *testing.T) {
	cases := []struct {
		input, wantName, wantVersion string
	}{
		{"firefox-128.0_1", "firefox", "128.0_1"},
		{"firefox-i18n-128.0_1", "firefox-i18n", "128.0_1"},
		{"0ad-0.27.1_2", "0ad", "0.27.1_2"},
		{"plain-name", "plain-name", ""},
	}
	for _, tc := range cases {
		name, version := splitNissaPackageVersion(tc.input)
		if name != tc.wantName || version != tc.wantVersion {
			t.Errorf("splitNissaPackageVersion(%q) = (%q, %q), want (%q, %q)",
				tc.input, name, version, tc.wantName, tc.wantVersion)
		}
	}
}

func TestNissaInstalledPackage(t *testing.T) {
	listing := "ii package-a-1.2_1 description\nii firefox-128.0_1 browser\n"
	if !nissaInstalledPackage("firefox", listing) {
		t.Fatal("expected firefox to be recognized as installed")
	}
	if nissaInstalledPackage("not-installed", listing) {
		t.Fatal("unexpected package match")
	}
}

func TestNissaVersionAndQueryLimits(t *testing.T) {
	if nissaVersion != 1 || nissaVersion2 != 2 || nissaMaxQueryLength != 512 {
		t.Fatalf("unexpected NISSA limits/version: v1=%d v2=%d query=%d", nissaVersion, nissaVersion2, nissaMaxQueryLength)
	}
}

func TestNissaPackageNameValidation(t *testing.T) {
	for _, name := range []string{"firefox", "0ad", "libfoo++", "python3.12", "foo_bar"} {
		if !nissaPackageName.MatchString(name) {
			t.Errorf("expected package name %q to be accepted", name)
		}
	}
	for _, name := range []string{"", "-R", "../tmp", "foo bar", "foo;rm"} {
		if nissaPackageName.MatchString(name) {
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
