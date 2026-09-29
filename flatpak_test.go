package main

import "testing"

func TestFlatpakCommandConfirmationAndRemote(t *testing.T) {
	old := yesFlag
	defer func() { yesFlag = old }()
	yesFlag = false
	got := flatpakCommand(FlatpakOptions{Remote: "gnome"}, "install", "org.gnome.Builder")
	if containsString(got, "-y") {
		t.Fatalf("unexpected automatic confirmation: %v", got)
	}
	if !containsString(got, "gnome") {
		t.Fatalf("remote not propagated: %v", got)
	}
	yesFlag = true
	got = flatpakCommand(FlatpakOptions{Remote: "gnome"}, "install", "org.gnome.Builder")
	if !containsString(got, "-y") {
		t.Fatalf("explicit confirmation was not propagated: %v", got)
	}
	got = flatpakCommand(FlatpakOptions{}, "remotes")
	if containsString(got, "-y") {
		t.Fatalf("query command received -y: %v", got)
	}
}

func TestParseRemoteColumns(t *testing.T) {
	remotes := parseRemoteColumns("flathub\tFlathub\thttps://dl.flathub.org/repo/\t\n")
	if len(remotes) != 1 || remotes[0].Name != "flathub" || remotes[0].URL == "" {
		t.Fatalf("unexpected remotes: %+v", remotes)
	}
}
