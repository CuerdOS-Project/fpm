package fpm

import (
	"bufio"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// Reference implementations: the original quadratic code the indexes replace.
// The optimized versions must agree with them on every input.

func naivePackageInInstalledList(pkg, output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == pkg {
			return true
		}
		if len(fields) > 0 && fields[0] == pkg {
			return true
		}
	}
	return false
}

func naivePlutoInstalledPackage(name, listing string) bool {
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		candidate := fields[0]
		if len(fields) > 1 && isInstalledListStatus(fields[0]) {
			candidate = fields[1]
		}
		candidateName, _ := splitPlutoPackageVersion(candidate)
		if candidateName == name || candidate == name {
			return true
		}
	}
	return false
}

func randomListing(rng *rand.Rand, lines int) (string, []string) {
	templates := []string{
		"ii pkg%d-1.0_1   summary words", "[*] pkg%d-2.0_1 summary", "[-] pkg%d-1.0_1 summary",
		"pkg%d-1.0_1", "uu pkg%d-1.0_1 x", "pkg%d", "org.flat.App%d", "ii pkg%d-1.0_1", "",
		"   ", "hr pkg%d-3.1_2 held", "ii   pkg%d-1.0_1\t tabbed summary", "[*]", "ii",
	}
	var b strings.Builder
	probes := []string{"", "ii", "[*]", "[-]", "nothing-here"}
	for i := 0; i < lines; i++ {
		id := rng.Intn(60)
		line := templates[rng.Intn(len(templates))]
		if strings.Contains(line, "%d") {
			line = fmt.Sprintf(line, id)
		}
		if rng.Intn(5) == 0 {
			line += "\r"
		}
		b.WriteString(line)
		b.WriteByte('\n')
		probes = append(probes, fmt.Sprintf("pkg%d", id), fmt.Sprintf("pkg%d-1.0_1", id), fmt.Sprintf("org.flat.App%d", id))
	}
	if rng.Intn(2) == 0 {
		b.WriteString("pkg7-1.0_1") // last line without newline
	}
	return b.String(), probes
}

func TestInstalledIndexesMatchTheOriginalLookups(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 200; round++ {
		listing, probes := randomListing(rng, rng.Intn(40))
		lineIndex := newInstalledLineIndex(listing)
		nameIndex := newInstalledNameIndex(listing)
		for _, probe := range probes {
			if got, want := lineIndex.has(probe), naivePackageInInstalledList(probe, listing); got != want {
				t.Fatalf("line index %q: got %v, want %v\nlisting:\n%s", probe, got, want, listing)
			}
			if got, want := nameIndex.has(probe), naivePlutoInstalledPackage(probe, listing); got != want {
				t.Fatalf("name index %q: got %v, want %v\nlisting:\n%s", probe, got, want, listing)
			}
			if packageInInstalledList(probe, listing) != naivePackageInInstalledList(probe, listing) ||
				plutoInstalledPackage(probe, listing) != naivePlutoInstalledPackage(probe, listing) {
				t.Fatalf("one-shot wrapper disagrees for %q", probe)
			}
		}
	}
}

func TestForEachLineMatchesBufioScanLines(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	alphabet := []string{"a", "bc", "\n", "\r", "\r\n", " "}
	for round := 0; round < 2000; round++ {
		var b strings.Builder
		for i, n := 0, rng.Intn(12); i < n; i++ {
			b.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		text := b.String()
		var want []string
		scanner := bufio.NewScanner(strings.NewReader(text))
		for scanner.Scan() {
			want = append(want, scanner.Text())
		}
		var got []string
		forEachLine(text, func(line string) bool {
			got = append(got, line)
			return true
		})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("forEachLine(%q) = %q, bufio.Scanner = %q", text, got, want)
		}
	}
}

func TestForEachLineStopsEarlyAndHasNoLineLimit(t *testing.T) {
	seen := 0
	forEachLine("a\nb\nc\n", func(string) bool {
		seen++
		return seen < 2
	})
	if seen != 2 {
		t.Fatalf("callback ran %d times, want 2", seen)
	}
	huge := strings.Repeat("x", 5<<20)
	var lengths []int
	forEachLine("first\n"+huge+"\nlast", func(line string) bool {
		lengths = append(lengths, len(line))
		return true
	})
	if !reflect.DeepEqual(lengths, []int{5, 5 << 20, 4}) {
		t.Fatalf("line lengths = %v", lengths)
	}
}

func TestContainsFoldMatchesToLowerContains(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	alphabet := []string{"a", "B", "c", "Z", "-", "_", " ", "1", "é", "É", "ß", "İ", "ı", "K"}
	random := func(max int) string {
		var b strings.Builder
		for i, n := 0, rng.Intn(max+1); i < n; i++ {
			b.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		return b.String()
	}
	for round := 0; round < 20000; round++ {
		s, needle := random(10), random(3)
		want := strings.Contains(strings.ToLower(s), strings.ToLower(needle))
		if got := containsFold(s, needle); got != want {
			t.Fatalf("containsFold(%q, %q) = %v, want %v", s, needle, got, want)
		}
	}
	for _, tc := range []struct {
		s, needle string
		want      bool
	}{
		{"A Widget Toolkit", "WIDGET", true}, {"A Widget Toolkit", "widget t", true},
		{"abc", "", true}, {"", "a", false}, {"ab", "abc", false}, {"Straße", "STRASSE", false},
	} {
		if got := containsFold(tc.s, tc.needle); got != tc.want {
			t.Errorf("containsFold(%q, %q) = %v, want %v", tc.s, tc.needle, got, tc.want)
		}
	}
}

func TestResolveUpdateDetailsKeepsOrderAndFallsBack(t *testing.T) {
	// pkg1 is missing from the listing: its version must come from the
	// per-package fallback (xbps-query -p pkgver), the rest from the listing.
	listing := "ii pkg0-1.0_1 s\nii pkg2-1.0_1 s\nii pkg3-1.0_1 s\n"
	stressFakeXBPS(t, listing, "", "")
	candidates := make([]xbpsUpdateCandidate, 0, 40)
	for i := 0; i < 40; i++ {
		candidates = append(candidates, xbpsUpdateCandidate{name: fmt.Sprintf("pkg%d", i%4), newVersion: "2.0_1"})
	}
	details := resolveUpdateDetails(candidates)
	if len(details) != len(candidates) {
		t.Fatalf("got %d details, want %d", len(details), len(candidates))
	}
	for i, detail := range details {
		if detail.current != "1.0_1" || detail.arch != "x86_64" {
			t.Fatalf("detail %d (%s) = %+v", i, candidates[i].name, detail)
		}
	}
	if got := resolveUpdateDetails(nil); len(got) != 0 {
		t.Fatalf("resolveUpdateDetails(nil) = %v", got)
	}
}

func TestParseXBPSUpdateCandidatesAcceptsWindowsLineEndings(t *testing.T) {
	got := parseXBPSUpdateCandidates("pkg1-2.0_1 update x86_64 repo 1 2\r\nname version\r\npkg2 3.1_1\r\n")
	want := []xbpsUpdateCandidate{{name: "pkg1", newVersion: "2.0_1"}, {name: "pkg2", newVersion: "3.1_1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
