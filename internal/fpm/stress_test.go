package fpm

// Stress tests: they feed FPM's data-heavy paths with huge synthetic inputs
// (package lists, search results, update lists, AM catalogs, SQLite
// snapshots) and check both speed and correctness. They guard against
// quadratic algorithms, silent truncation of very long lines and
// one-process-per-package loops.
//
//   go test ./internal/fpm -run Stress -v                 (N = 5000, ~seconds)
//   FPM_STRESS_N=50000 go test ./internal/fpm -run Stress -v
//   FPM_STRESS=1 go test ./internal/fpm -run Stress -v     (N = 200000)
//
// Every test logs elapsed time and allocations so regressions are visible
// even when the generous budgets still pass.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const stressDefaultN = 5000

func stressN(t *testing.T) int {
	t.Helper()
	if value := strings.TrimSpace(os.Getenv("FPM_STRESS_N")); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 100 {
			t.Fatalf("FPM_STRESS_N must be an integer >= 100, got %q", value)
		}
		return n
	}
	if os.Getenv("FPM_STRESS") == "1" {
		return 200000
	}
	return stressDefaultN
}

// stressSpawnN scales the tests that start one external process per item.
func stressSpawnN(n int) int {
	switch u := n / 25; {
	case u < 100:
		return 100
	case u > 3000:
		return 3000
	default:
		return u
	}
}

// stressBudget is deliberately generous (a linear algorithm needs a small
// fraction of it) so only super-linear behaviour trips it.
func stressBudget(n int) time.Duration {
	return 2*time.Second + time.Duration(n)*50*time.Microsecond
}

type stressResult struct {
	elapsed time.Duration
	alloc   uint64
	mallocs uint64
}

func stressMeasure(t *testing.T, label string, fn func()) stressResult {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	fn()
	result := stressResult{elapsed: time.Since(start)}
	runtime.ReadMemStats(&after)
	result.alloc = after.TotalAlloc - before.TotalAlloc
	result.mallocs = after.Mallocs - before.Mallocs
	t.Logf("%-40s %10s  alloc=%8.1f MiB  mallocs=%d",
		label, result.elapsed.Round(time.Millisecond), float64(result.alloc)/(1<<20), result.mallocs)
	return result
}

func stressWithin(t *testing.T, label string, result stressResult, budget time.Duration) {
	t.Helper()
	if result.elapsed > budget {
		t.Errorf("%s took %s, budget %s: the algorithm does not scale", label, result.elapsed.Round(time.Millisecond), budget)
	}
}

// stressJunkLine is longer than bufio.Scanner's 1 MiB token limit and is not
// a valid package line; parsers must skip it and keep going.
func stressJunkLine() string { return "#" + strings.Repeat("x", 3<<20) }

// xbps-query -l: "ii name-version   summary", installed only for even i.
func stressInstalledListing(n int, onlyEven bool) string {
	var b strings.Builder
	b.Grow(n * 60)
	for i := 0; i < n; i++ {
		if onlyEven && i%2 != 0 {
			continue
		}
		fmt.Fprintf(&b, "ii pkg%d-1.0_1   Summary of package %d with Some Words\n", i, i)
	}
	return b.String()
}

// xbps-query -Rs: n unique names, every even one repeated by a second repo.
func stressSearchOutput(n int, junkAt int) string {
	var b strings.Builder
	b.Grow(n * 70)
	for i := 0; i < n; i++ {
		if i == junkAt {
			b.WriteString(stressJunkLine())
			b.WriteByte('\n')
		}
		mark := "[-]"
		if i%3 == 0 {
			mark = "[*]"
		}
		fmt.Fprintf(&b, "%s pkg%d-1.0_1 Summary %d\n", mark, i, i)
		if i%2 == 0 {
			fmt.Fprintf(&b, "[-] pkg%d-1.1_1 Summary %d (second repository)\n", i, i)
		}
	}
	return b.String()
}

func stressPackageIndex(t *testing.T, name string) int {
	t.Helper()
	index, err := strconv.Atoi(strings.TrimPrefix(name, "pkg"))
	if err != nil {
		t.Fatalf("unexpected package name %q", name)
	}
	return index
}

func TestStressSearchRowsAgainstHugeInstalledList(t *testing.T) {
	n := stressN(t)
	listing := stressInstalledListing(n, true)
	for _, junkAt := range []int{-1, n / 2} {
		name := "plain"
		if junkAt >= 0 {
			name = "with 3 MiB junk line in the middle"
		}
		output := stressSearchOutput(n, junkAt)
		var rows []xbpsRow
		result := stressMeasure(t, fmt.Sprintf("parseXBPSSearchRows n=%d (%s)", n, name), func() {
			rows = parseXBPSSearchRows(output, listing, true, -1)
		})
		stressWithin(t, name, result, stressBudget(n))
		if len(rows) != n {
			t.Fatalf("%s: got %d rows, want %d (rows lost after a long line?)", name, len(rows), n)
		}
		for _, row := range rows {
			i := stressPackageIndex(t, row.Name)
			want := i%2 == 0 || i%3 == 0
			if row.Installed != want {
				t.Fatalf("%s: %s installed=%v, want %v", name, row.Name, row.Installed, want)
			}
		}
	}
}

func TestStressParseSearchLinesAgainstHugeInstalledList(t *testing.T) {
	n := stressN(t)
	listing := stressInstalledListing(n, true)
	output := stressSearchOutput(n, -1)
	var items []TuiItem
	result := stressMeasure(t, fmt.Sprintf("parseSearchLines XBPS n=%d", n), func() {
		items = parseSearchLines(output, "XBPS", listing)
	})
	stressWithin(t, "parseSearchLines XBPS", result, stressBudget(n))
	wantLines := strings.Count(strings.TrimSpace(output), "\n") + 1
	if len(items) != wantLines {
		t.Fatalf("got %d items, want %d", len(items), wantLines)
	}
	for _, item := range items {
		base, _ := splitPlutoPackageVersion(item.Name)
		// Only the exact pkgver of the even packages is in the listing.
		want := stressPackageIndex(t, base)%2 == 0 && item.Name == base+"-1.0_1"
		if item.Installed != want {
			t.Fatalf("%s installed=%v, want %v", item.Name, item.Installed, want)
		}
	}

	var flat strings.Builder
	var installed strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&flat, "App %d\tDescription %d\torg.example.App%d\tflathub\n", i, i, i)
		if i%2 == 0 {
			fmt.Fprintf(&installed, "org.example.App%d\n", i)
		}
	}
	result = stressMeasure(t, fmt.Sprintf("parseSearchLines tab n=%d", n), func() {
		items = parseSearchLines(flat.String(), "Flatpak", installed.String())
	})
	stressWithin(t, "parseSearchLines tab", result, stressBudget(n))
	if len(items) != n {
		t.Fatalf("got %d flatpak items, want %d", len(items), n)
	}
}

func TestStressInstalledXBPSWithManyArguments(t *testing.T) {
	n := stressN(t)
	stressFakeXBPS(t, stressInstalledListing(n, true), "", "")
	packages := make([]string, 0, n)
	for i := 0; i < n; i++ {
		packages = append(packages, fmt.Sprintf("pkg%d-1.0_1", i))
	}
	var installed []string
	result := stressMeasure(t, fmt.Sprintf("installedXBPS %d args vs %d listed", n, n/2), func() {
		installed = installedXBPS(packages)
	})
	stressWithin(t, "installedXBPS", result, stressBudget(n))
	if len(installed) != (n+1)/2 {
		t.Fatalf("got %d installed, want %d", len(installed), (n+1)/2)
	}
}

func TestStressUpdateCandidatesSurviveHugeLines(t *testing.T) {
	n := stressN(t)
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i == n/2 {
			b.WriteString(stressJunkLine())
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "pkg%d-2.0_1 update x86_64 https://repo.example/current 1024 2048\n", i)
	}
	var candidates []xbpsUpdateCandidate
	result := stressMeasure(t, fmt.Sprintf("parseXBPSUpdateCandidates n=%d", n), func() {
		candidates = parseXBPSUpdateCandidates(b.String())
	})
	stressWithin(t, "parseXBPSUpdateCandidates", result, stressBudget(n))
	if len(candidates) != n {
		t.Fatalf("got %d candidates, want %d: updates after a very long line were silently dropped", len(candidates), n)
	}
}

func TestStressDedupe(t *testing.T) {
	n := stressN(t)
	repos := []string{"nonfree", "multilib", "cuerdos", "current", "bogus"}
	rows := make([]xbpsRow, 0, n*len(repos))
	items := make([]TuiItem, 0, n*3)
	for i := 0; i < n; i++ {
		for r, repo := range repos {
			rows = append(rows, xbpsRow{Name: fmt.Sprintf("pkg%d", i), Version: "1.0_1", Summary: "s", Repo: repo})
			if r < 3 {
				items = append(items, TuiItem{Name: fmt.Sprintf("pkg%d-%d.0_1", i, r+1), Source: "XBPS"})
			}
		}
	}
	var dedupedRows []xbpsRow
	result := stressMeasure(t, fmt.Sprintf("dedupeXBPSRows %d rows", len(rows)), func() {
		dedupedRows = dedupeXBPSRows(rows)
	})
	stressWithin(t, "dedupeXBPSRows", result, stressBudget(len(rows)))
	if len(dedupedRows) != n {
		t.Fatalf("got %d rows, want %d", len(dedupedRows), n)
	}
	for _, row := range dedupedRows {
		if row.Repo != "current" {
			t.Fatalf("%s kept repository %q, want current", row.Name, row.Repo)
		}
	}
	var dedupedItems []TuiItem
	result = stressMeasure(t, fmt.Sprintf("dedupeXBPSItems %d items", len(items)), func() {
		dedupedItems = dedupeXBPSItems(items)
	})
	stressWithin(t, "dedupeXBPSItems", result, stressBudget(len(items)))
	if len(dedupedItems) != n {
		t.Fatalf("got %d items, want %d", len(dedupedItems), n)
	}
}

// Typing in the TUI filter runs itemMatches over every item on each keystroke.
func TestStressFilterDoesNotAllocatePerItem(t *testing.T) {
	n := stressN(t)
	items := make([]TuiItem, n)
	for i := range items {
		items[i] = TuiItem{Name: fmt.Sprintf("libfoo%d", i), Label: fmt.Sprintf("libfoo%d", i), Desc: fmt.Sprintf("A Widget Toolkit number %d", i)}
	}
	queries := []string{"w", "wi", "wid", "widg", "widge", "widget", "widget t", "WIDGET TOOLKIT"}
	matches := 0
	result := stressMeasure(t, fmt.Sprintf("itemMatches %d items x %d keystrokes", n, len(queries)), func() {
		for _, query := range queries {
			for _, item := range items {
				if itemMatches(item, query) {
					matches++
				}
			}
		}
	})
	stressWithin(t, "itemMatches", result, stressBudget(n*len(queries)))
	if matches != n*len(queries) {
		t.Fatalf("matches = %d, want %d", matches, n*len(queries))
	}
	allocs := testing.AllocsPerRun(100, func() { itemMatches(items[0], "widget") })
	if allocs != 0 {
		t.Errorf("itemMatches allocates %.0f times per item; filtering %d items per keystroke becomes GC-bound", allocs, n)
	}
}

func TestStressSnapshotDatabaseRoundTrip(t *testing.T) {
	n := stressN(t)
	listing := stressInstalledListing(n, false)
	var packages []cachedPackage
	result := stressMeasure(t, fmt.Sprintf("parseInstalledPackageSnapshot n=%d", n), func() {
		packages = parseInstalledPackageSnapshot(listing)
	})
	stressWithin(t, "parseInstalledPackageSnapshot", result, stressBudget(n))
	if len(packages) != n {
		t.Fatalf("parsed %d packages, want %d", len(packages), n)
	}

	path := filepath.Join(t.TempDir(), "fpm.db")
	result = stressMeasure(t, fmt.Sprintf("writePackageSnapshotAt n=%d", n), func() {
		if err := writePackageSnapshotAt(path, packages, time.Now().UTC(), os.Geteuid()); err != nil {
			t.Fatal(err)
		}
	})
	stressWithin(t, "writePackageSnapshotAt", result, stressBudget(n)+2*time.Second)

	var loaded []cachedPackage
	result = stressMeasure(t, fmt.Sprintf("loadFPMDatabaseSnapshotAt n=%d", n), func() {
		var err error
		loaded, _, err = loadFPMDatabaseSnapshotAt(path)
		if err != nil {
			t.Fatal(err)
		}
	})
	stressWithin(t, "loadFPMDatabaseSnapshotAt", result, stressBudget(n))
	if len(loaded) != n {
		t.Fatalf("loaded %d packages, want %d", len(loaded), n)
	}

	old := fpmDBPath
	fpmDBPath = path
	defer func() { fpmDBPath = old }()
	var count int
	info := stressMeasure(t, "packageDatabaseInfo (only needs a count)", func() {
		var err error
		count, _, _, err = packageDatabaseInfo()
		if err != nil {
			t.Fatal(err)
		}
	})
	if count != n {
		t.Fatalf("packageDatabaseInfo count = %d, want %d", count, n)
	}
	if info.mallocs > uint64(n/10)+1000 {
		t.Errorf("packageDatabaseInfo made %d allocations for a COUNT; it loads every package row", info.mallocs)
	}
}

func TestStressAMFusionCatalog(t *testing.T) {
	n := stressN(t)
	var catalogText, installedText strings.Builder
	for i := 0; i < n; i++ {
		if i%7 == 0 {
			fmt.Fprintf(&catalogText, "\x1b[1;32m◆ app%d\x1b[0m : Description number %d\n", i, i)
		} else {
			fmt.Fprintf(&catalogText, "◆ app%d : Description number %d\n", i, i)
		}
		if i%5 == 0 {
			fmt.Fprintf(&installedText, "◆ app%d | 1.%d | appimage | 10 MiB\n", i, i)
		}
	}
	var catalog []amFusionPackage
	result := stressMeasure(t, fmt.Sprintf("parseAMFusionCatalog n=%d", n), func() {
		catalog = parseAMFusionCatalog(catalogText.String())
	})
	stressWithin(t, "parseAMFusionCatalog", result, stressBudget(n))
	if len(catalog) != n {
		t.Fatalf("catalog has %d entries, want %d", len(catalog), n)
	}
	var installed map[string]amFusionPackage
	result = stressMeasure(t, fmt.Sprintf("parseAMFusionInstalled n=%d", n/5), func() {
		installed = parseAMFusionInstalled(installedText.String())
	})
	stressWithin(t, "parseAMFusionInstalled", result, stressBudget(n))
	if len(installed) != (n+4)/5 {
		t.Fatalf("installed has %d entries, want %d", len(installed), (n+4)/5)
	}

	old := amFusionDBPathOverride
	amFusionDBPathOverride = filepath.Join(t.TempDir(), "am-fusion.db")
	defer func() { amFusionDBPathOverride = old }()
	result = stressMeasure(t, fmt.Sprintf("saveAMFusionSnapshot n=%d", n), func() {
		if err := saveAMFusionSnapshot(catalog, installed); err != nil {
			t.Fatal(err)
		}
	})
	stressWithin(t, "saveAMFusionSnapshot", result, stressBudget(n)+2*time.Second)
	result = stressMeasure(t, fmt.Sprintf("cacheAMFusionCatalog n=%d", n), func() {
		if err := cacheAMFusionCatalog(catalog); err != nil {
			t.Fatal(err)
		}
	})
	stressWithin(t, "cacheAMFusionCatalog", result, stressBudget(n)+2*time.Second)
	var index map[string]amFusionPackage
	result = stressMeasure(t, fmt.Sprintf("amFusionPackageIndex n=%d", n), func() {
		var err error
		index, err = amFusionPackageIndex()
		if err != nil {
			t.Fatal(err)
		}
	})
	stressWithin(t, "amFusionPackageIndex", result, stressBudget(n))
	if len(index) != n {
		t.Fatalf("index has %d entries, want %d", len(index), n)
	}
}

// ---- PLUTO end to end, with a fake xbps -------------------------------------

// stressFakeXBPS installs fake xbps-query/xbps-install scripts first in PATH
// and returns the path of a log with one line per invocation.
func stressFakeXBPS(t *testing.T, listing, updates, search string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"listing": listing, "updates": updates, "search": search} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	logPath := filepath.Join(dir, "calls.log")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo "$*" >> "` + logPath + `"`,
		`case "$1" in`,
		`-l) if [ $# -eq 1 ]; then cat "` + dir + `/listing"; fi ;;`,
		`-Rs) cat "` + dir + `/search" ;;`,
		`-un) cat "` + dir + `/updates" ;;`,
		`-p) case "$2" in pkgver) echo "$3-1.0_1" ;; architecture) echo x86_64 ;; esac ;;`,
		`esac`,
		``,
	}, "\n")
	for _, name := range []string{"xbps-query", "xbps-install"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// stressCaptureStdout redirects stdout to a file (a pipe would deadlock once
// the output exceeds its 64 KiB buffer) and returns the file path.
func stressCaptureStdout(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = file
	code := func() int {
		defer func() { os.Stdout = previous }()
		return fn()
	}()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path, code
}

func stressCountLines(t *testing.T, path, needle string) (total, matching int) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		total++
		if strings.Contains(scanner.Text(), needle) {
			matching++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return total, matching
}

func stressCountCalls(t *testing.T, logPath, prefix string) int {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && strings.HasPrefix(line, prefix) {
			count++
		}
	}
	return count
}

func TestStressPlutoInstalledStream(t *testing.T) {
	n := stressN(t)
	half := stressInstalledListing(n/2, false)
	full := stressInstalledListing(n, false)
	listing := half + stressJunkLine() + "\n" + full[len(half):]
	stressFakeXBPS(t, listing, "", "")
	var path string
	var code int
	result := stressMeasure(t, fmt.Sprintf("pluto v2 installed n=%d (+3 MiB line)", n), func() {
		path, code = stressCaptureStdout(t, func() int { return plutoInstalled(nil) })
	})
	stressWithin(t, "plutoInstalled", result, stressBudget(n)*2)
	if code != 0 {
		t.Fatalf("plutoInstalled exit code = %d, want 0 (a very long line aborted the whole query)", code)
	}
	_, results := stressCountLines(t, path, `"installed.result"`)
	if results != n {
		t.Fatalf("emitted %d installed.result records, want %d", results, n)
	}
	_, done := stressCountLines(t, path, `"installed.done"`)
	if done != 1 {
		t.Fatalf("installed.done emitted %d times, want 1", done)
	}
}

func TestStressPlutoUpdatesDoNotSpawnProcessesPerPackage(t *testing.T) {
	n := stressSpawnN(stressN(t))
	var updates strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&updates, "pkg%d-2.0_1 update x86_64 https://repo.example/current 1024 2048\n", i)
	}
	logPath := stressFakeXBPS(t, stressInstalledListing(n, false), updates.String(), "")
	var path string
	var code int
	result := stressMeasure(t, fmt.Sprintf("pluto v2 updates n=%d", n), func() {
		path, code = stressCaptureStdout(t, func() int { return plutoUpdates(nil) })
	})
	if code != 0 {
		t.Fatalf("plutoUpdates exit code = %d", code)
	}
	_, results := stressCountLines(t, path, `"updates.result"`)
	if results != n {
		t.Fatalf("emitted %d updates.result records, want %d", results, n)
	}
	_, current := stressCountLines(t, path, `"current_version":"1.0_1"`)
	_, arch := stressCountLines(t, path, `"arch":"x86_64"`)
	if current != n || arch != n {
		t.Fatalf("records with correct version/arch = %d/%d, want %d", current, arch, n)
	}
	calls := stressCountCalls(t, logPath, "")
	t.Logf("external xbps processes started: %d for %d updates", calls, n)
	if calls > n+3 {
		t.Errorf("%d external processes for %d updates (%.1f per package); fork/exec per package does not scale", calls, n, float64(calls)/float64(n))
	}
	stressWithin(t, "plutoUpdates", result, stressBudget(n)*20)
}
