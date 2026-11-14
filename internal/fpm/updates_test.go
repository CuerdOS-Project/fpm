package fpm

import "testing"

func TestParseXBPSUpdateCandidates(t *testing.T) {
	output := "Name Action Version\n" +
		"firefox-135.0_1 update\n" +
		"libreoffice 25.2.0_1 x86_64\n" +
		"--------------------------------\n" +
		"not-a-version update\n" +
		"firefox-135.0_1 update duplicate\n"

	got := parseXBPSUpdateCandidates(output)
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2: %#v", len(got), got)
	}
	if got[0].name != "firefox" || got[0].newVersion != "135.0_1" {
		t.Fatalf("unexpected first candidate: %#v", got[0])
	}
	if got[1].name != "libreoffice" || got[1].newVersion != "25.2.0_1" {
		t.Fatalf("unexpected second candidate: %#v", got[1])
	}
}

func TestParseXBPSUpdateCandidatesEmptyAndMalformed(t *testing.T) {
	if got := parseXBPSUpdateCandidates("Name Action Version\n---\nfoo update\n"); len(got) != 0 {
		t.Fatalf("expected no candidates for malformed input, got %#v", got)
	}
}
