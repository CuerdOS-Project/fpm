package main

import "testing"

func TestCanonicalCommandAliases(t *testing.T) {
	cases := map[string]string{
		"flat": "flatpak", "flatpak": "flatpak",
		"aimg": "appimage", "appimage": "appimage",
		"install": "install",
	}
	for input, want := range cases {
		if got := canonicalCommand(input); got != want {
			t.Errorf("canonicalCommand(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseFlatpakCLIOptionsBeforeAndAfterSubcommand(t *testing.T) {
	cases := []struct {
		args []string
	}{
		{args: []string{"--user", "install", "--remote", "flathub", "org.gimp.GIMP"}},
		{args: []string{"install", "--user", "--remote", "flathub", "org.gimp.GIMP"}},
	}
	for _, tc := range cases {
		subcommand, options, positional := parseFlatpakCLI(tc.args)
		if subcommand != "install" || !options.User || options.Remote != "flathub" || len(positional) != 1 || positional[0] != "org.gimp.GIMP" {
			t.Errorf("unexpected parse for %v: subcommand=%q options=%+v positional=%v", tc.args, subcommand, options, positional)
		}
	}
}

func TestParseAppImageArgs(t *testing.T) {
	options, positional, err := parseAppImageArgs([]string{
		"--name", "Mi App", "--desc=Editor de imágenes", "--category", "Graphics;",
		"--icon", "/tmp/icon.png", "--plain", "demo.AppImage",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.Name != "Mi App" || options.Desc != "Editor de imágenes" || options.Category != "Graphics;" || options.Icon != "/tmp/icon.png" || !options.Plain {
		t.Fatalf("unexpected options: %+v", options)
	}
	if len(positional) != 1 || positional[0] != "demo.AppImage" {
		t.Fatalf("unexpected positional args: %v", positional)
	}
}

func TestParseAppImageArgsInteractive(t *testing.T) {
	options, positional, err := parseAppImageArgs([]string{"--interactive", "demo.AppImage"})
	if err != nil || !options.Interactive || len(positional) != 1 {
		t.Fatalf("unexpected result: %+v %v %v", options, positional, err)
	}
}

func TestParseAppImageArgsErrors(t *testing.T) {
	if _, _, err := parseAppImageArgs([]string{"--name"}); err == nil {
		t.Fatal("expected missing value error")
	}
	if _, _, err := parseAppImageArgs([]string{"--unknown", "demo.AppImage"}); err == nil {
		t.Fatal("expected unknown option error")
	}
}
