package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseInstalledPackageSnapshot(t *testing.T) {
	got := parseInstalledPackageSnapshot(strings.Join([]string{
		"ii firefox-128.0_1 Fast web browser",
		"ii libfoo++-2.1_3 C++ utility library",
		"[-] not-installed-1.0 Description",
		"ii malformed-no-version Description",
		"ii firefox-128.0_1 Duplicate should be ignored",
		"broken",
	}, "\n"))
	want := []cachedPackage{
		{Name: "firefox", Version: "128.0_1", Summary: "Fast web browser"},
		{Name: "libfoo++", Version: "2.1_3", Summary: "C++ utility library"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseInstalledPackageSnapshot() = %#v, want %#v", got, want)
	}
}

func TestFPMDatabaseSnapshotCreateReplaceAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fpm.db")
	syncedAt := time.Date(2026, 9, 27, 7, 0, 0, 0, time.UTC)
	first := []cachedPackage{{Name: "bash", Version: "5.2_1"}, {Name: "firefox", Version: "128.0_1", Summary: "Browser"}}
	if err := writePackageSnapshotAt(path, first, syncedAt, os.Geteuid()); err != nil {
		t.Fatal(err)
	}
	got, stamp, err := loadFPMDatabaseSnapshotAt(path)
	if err != nil {
		t.Fatal(err)
	}
	firstStamp := syncedAt.Format(time.RFC3339Nano)
	for i := range first {
		first[i].LastSeenAt = firstStamp
	}
	if !reflect.DeepEqual(got, first) || stamp != firstStamp {
		t.Fatalf("read snapshot = %#v, %q; want %#v, %q", got, stamp, first, syncedAt.Format(time.RFC3339Nano))
	}
	second := []cachedPackage{{Name: "bash", Version: "5.2_2"}}
	secondSync := syncedAt.Add(time.Minute)
	if err := writePackageSnapshotAt(path, second, secondSync, os.Geteuid()); err != nil {
		t.Fatal(err)
	}
	got, stamp, err = loadFPMDatabaseSnapshotAt(path)
	second[0].LastSeenAt = secondSync.Format(time.RFC3339Nano)
	if err != nil || !reflect.DeepEqual(got, second) || stamp != secondSync.Format(time.RFC3339Nano) {
		t.Fatalf("replacement snapshot = %#v, err %v; want %#v", got, err, second)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("database mode = %o, want 644", info.Mode().Perm())
	}
}

func TestInvalidFPMDatabaseSnapshotDoesNotDestroyPreviousData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fpm.db")
	good := []cachedPackage{{Name: "bash", Version: "5.2_1"}}
	syncedAt := time.Now().UTC()
	if err := writePackageSnapshotAt(path, good, syncedAt, os.Geteuid()); err != nil {
		t.Fatal(err)
	}
	good[0].LastSeenAt = syncedAt.Format(time.RFC3339Nano)
	if err := writePackageSnapshotAt(path, []cachedPackage{{Name: "../bad", Version: "1"}}, time.Now(), os.Geteuid()); err == nil {
		t.Fatal("invalid package snapshot unexpectedly succeeded")
	}
	got, _, err := loadFPMDatabaseSnapshotAt(path)
	if err != nil || !reflect.DeepEqual(got, good) {
		t.Fatalf("failed snapshot damaged prior DB: got %#v, err %v", got, err)
	}
}

func TestFPMDatabasePathIsSeparateFromXBPSDatabase(t *testing.T) {
	if fpmDBPath == "/var/db/xbps" || fpmDBPath == "/var/db/xbps/pkgdb-0.38.plist" {
		t.Fatalf("FPM snapshot must not replace XBPS data: %q", fpmDBPath)
	}
	if fpmDBPath != defaultFPMDBPath {
		t.Fatalf("unexpected default FPM database path %q", fpmDBPath)
	}
}

func TestDBListDoesNotRequireRoot(t *testing.T) {
	if dbCommandNeedsRoot([]string{"list"}) {
		t.Fatal("db list should be readable without root")
	}
	for _, args := range [][]string{nil, {}, {"sync"}, {"check"}, {"repair"}} {
		if !dbCommandNeedsRoot(args) {
			t.Errorf("db %v should require root", args)
		}
	}
}

func TestFPMDatabaseRejectsUnsafeDirectories(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(parent, 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0777); err != nil {
		t.Fatal(err)
	}
	if err := prepareFPMDBPath(filepath.Join(parent, "fpm.db"), os.Geteuid()); err == nil {
		t.Fatal("unsafe writable directory unexpectedly accepted")
	}
}

func TestInstalledListStatus(t *testing.T) {
	for _, status := range []string{"ii", "[*]", "[**]"} {
		if !isInstalledListStatus(status) {
			t.Errorf("expected %q to mean installed", status)
		}
	}
	for _, status := range []string{"[-]", "[ ]", "rc", ""} {
		if isInstalledListStatus(status) {
			t.Errorf("expected %q to mean not installed", status)
		}
	}
}
