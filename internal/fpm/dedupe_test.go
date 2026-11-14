package fpm

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestDedupeXBPSRowsPrefersInstalledThenRepoThenRichness(t *testing.T) {
	rows := dedupeXBPSRows([]xbpsRow{
		{Name: "vim", Version: "9.1", Repo: "nonfree"},
		{Name: "vim", Version: "9.1", Repo: "current"},
		{Name: "nano", Version: "7"},
		{Name: "git", Repo: "current"},
		{Name: "git", Version: "2.45", Summary: "dvcs", Repo: "current"},
		{Name: "htop", Version: "3", Repo: "current"},
		{Name: "htop", Version: "3", Repo: "multilib", Installed: true},
	})
	var names []string
	for _, r := range rows {
		names = append(names, r.Name)
	}
	if !reflect.DeepEqual(names, []string{"vim", "nano", "git", "htop"}) {
		t.Fatalf("order/uniqueness wrong: %v", names)
	}
	if rows[0].Repo != "current" {
		t.Errorf("vim should come from current, got %q", rows[0].Repo)
	}
	if rows[2].Summary != "dvcs" {
		t.Errorf("git should keep the richer row: %+v", rows[2])
	}
	if !rows[3].Installed {
		t.Errorf("installed htop row must win: %+v", rows[3])
	}
}

func TestParseXBPSSearchRowsCollapsesVersionsAndLimits(t *testing.T) {
	out := "[-] foo-1.0_1 first\n[*] foo-1.1_1 second\n[-] bar-2_1 bar\n[-] baz-3_1 baz\n"
	rows := parseXBPSSearchRows(out, "", false, 2)
	if len(rows) != 2 || rows[0].Name != "foo" || !rows[0].Installed || rows[0].Version != "1.1_1" || rows[1].Name != "bar" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	rows = parseXBPSSearchRows("[-] foo-1.0_1 d\n", "ii foo-1.0_1 d\n", true, -1)
	if len(rows) != 1 || !rows[0].Installed {
		t.Fatalf("installed state from xbps-query -l lost: %+v", rows)
	}
}

func TestDedupeXBPSItemsLeavesOtherSourcesAlone(t *testing.T) {
	items := dedupeXBPSItems([]TuiItem{
		{Name: "foo-1.0_1", Source: "XBPS"},
		{Name: "foo-1.1_1", Source: "XBPS", Installed: true},
		{Name: "org.x.Foo", Source: "Flatpak"},
		{Name: "org.x.Foo", Source: "Flatpak"},
	})
	if len(items) != 3 || !items[0].Installed {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestPlutoV1SearchEmitsOneRowPerPackage(t *testing.T) {
	dir := t.TempDir()
	fakeExecutable(t, dir, "xbps-query", `case "$1" in
-Rs) printf '[-] foo-1.0_1 old\n[-] foo-1.1_1 new\n[-] bar-2_1 bar\n';;
-l) printf 'ii foo-1.1_1 new\n';;
esac`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, code := captureStdout(t, func() int { return runPluto([]string{"v1", "search", "foo"}) })
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], `"name":"foo"`) || !strings.Contains(lines[0], `"installed":true`) ||
		!strings.Contains(lines[2], `"count":2`) {
		t.Fatalf("unexpected stream:\n%s", out)
	}
}
