package fpm

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAMFusionCatalog(t *testing.T) {
	output := "Search results:\n\x1b[32m◆ Firefox-AppImage : Fast web browser\x1b[0m\n◆ Krita.AppImage : Digital painting\n◆ invalid name : ignored\n◆ Firefox-AppImage : duplicate\n"
	got := parseAMFusionCatalog(output)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique valid catalog rows, got %+v", got)
	}
	if got[0].Name != "Firefox-AppImage" || got[0].Description != "Fast web browser" {
		t.Fatalf("unexpected first row: %+v", got[0])
	}
	if got[1].Name != "Krita.AppImage" || got[1].Type != "appimage" {
		t.Fatalf("unexpected second row: %+v", got[1])
	}
}

func TestParseAMFusionInstalled(t *testing.T) {
	output := "- APPNAME | VERSION | TYPE | SIZE\n- ------- | ------- | ---- | ----\n◆ Firefox-AppImage | 128.1 | appimage | 90.0MiB\n◆ Krita | custom-db | 5.2.0 | appimage🔒 | 250MiB\n◆ LibreOffice | 24.2 | static-binary | 80MiB\n"
	got := parseAMFusionInstalled(output)
	if len(got) != 2 {
		t.Fatalf("expected only AppImages, got %+v", got)
	}
	if got["Firefox-AppImage"].Version != "128.1" || !got["Firefox-AppImage"].Installed || got["Firefox-AppImage"].SizeBytes != 94371840 {
		t.Fatalf("unexpected Firefox inventory row: %+v", got["Firefox-AppImage"])
	}
	if got["Krita"].Version != "5.2.0" || got["Krita"].SizeBytes != 262144000 {
		t.Fatalf("unexpected custom repository version: %+v", got["Krita"])
	}
}

func TestAMFusionDatabaseStoresAMAndLocalAppImages(t *testing.T) {
	oldPath := amFusionDBPathOverride
	amFusionDBPathOverride = filepath.Join(t.TempDir(), "am-fusion.db")
	t.Cleanup(func() { amFusionDBPathOverride = oldPath })

	catalog := []amFusionPackage{{Name: "Firefox", Description: "Web browser", Type: "appimage"}}
	installed := map[string]amFusionPackage{
		"Firefox": {Name: "Firefox", Version: "128.1", Type: "appimage", Installed: true},
	}
	if err := saveAMFusionSnapshot(catalog, installed); err != nil {
		t.Fatal(err)
	}
	if pkg, found, err := findAMFusionPackage("firefox"); err != nil || !found || !pkg.Installed {
		t.Fatalf("AM package lookup failed: pkg=%+v found=%t err=%v", pkg, found, err)
	}
	if err := syncFPMLocalAppImages([]InstalledApp{{Slug: "Firefox", Name: "Firefox local", Category: "Network;", AppImagePath: "/tmp/Firefox.AppImage"}}); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", amFusionDBPathOverride)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM appimage_packages WHERE name='Firefox'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected same package name in AM and FPM-local sources, got %d rows", count)
	}
	var version string
	var installedFlag int
	if err := db.QueryRow(`SELECT version, installed FROM appimage_packages WHERE repository=? AND name='Firefox'`, amFusionRepoName).Scan(&version, &installedFlag); err != nil {
		t.Fatal(err)
	}
	if version != "128.1" || installedFlag != 1 {
		t.Fatalf("AM row not synchronized: version=%q installed=%d", version, installedFlag)
	}
	var path string
	if err := db.QueryRow(`SELECT install_path FROM appimage_packages WHERE repository='FPM-local' AND name='Firefox'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/Firefox.AppImage" {
		t.Fatalf("unexpected local AppImage path: %q", path)
	}
}

func TestAMFusionRepositoryCanBeDisabledAndEnabled(t *testing.T) {
	oldPath := amFusionDBPathOverride
	amFusionDBPathOverride = filepath.Join(t.TempDir(), "am-fusion.db")
	t.Cleanup(func() { amFusionDBPathOverride = oldPath })

	if enabled, err := amFusionBackendEnabled(); err != nil || enabled {
		t.Fatalf("repository should start disabled until installed: enabled=%t err=%v", enabled, err)
	}
	if err := setAMFusionEnabled(true); err != nil {
		t.Fatal(err)
	}
	if enabled, err := amFusionBackendEnabled(); err != nil || !enabled {
		t.Fatalf("repository should be enabled after activation: enabled=%t err=%v", enabled, err)
	}
	if err := setAMFusionEnabled(false); err != nil {
		t.Fatal(err)
	}
	if enabled, err := amFusionBackendEnabled(); err != nil || enabled {
		t.Fatalf("repository should be disabled: enabled=%t err=%v", enabled, err)
	}
	if err := validateAMFusionRepoName(amFusionRepoName); err != nil {
		t.Fatalf("disabled source should remain manageable by name: %v", err)
	}
	if err := setAMFusionEnabled(true); err != nil {
		t.Fatal(err)
	}
	if err := validateAMFusionRepo(amFusionRepoName); err != nil {
		t.Fatalf("enabled repository should validate: %v", err)
	}
}

func TestParseAMFusionSize(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"0B", 0}, {"512 B", 512}, {"1KiB", 1024}, {"1.5MiB", 1572864},
		{"2GiB", 2147483648}, {"1KB", 1000}, {"1.25MB", 1250000}, {"3GB", 3000000000},
		{"invalid", 0}, {"1TiB", 0}, {"999999999999999999999GiB", 0}, {"-1MiB", 0},
	}
	for _, tc := range cases {
		if got := parseAMFusionSize(tc.input); got != tc.want {
			t.Errorf("parseAMFusionSize(%q)=%d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestAMFusionSizeSchemaMigration(t *testing.T) {
	oldPath := amFusionDBPathOverride
	amFusionDBPathOverride = filepath.Join(t.TempDir(), "old-am-fusion.db")
	t.Cleanup(func() { amFusionDBPathOverride = oldPath })
	db, err := sql.Open("sqlite", amFusionDBPathOverride)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE appimage_packages (
id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
version TEXT NOT NULL DEFAULT '', package_type TEXT NOT NULL DEFAULT 'appimage',
installed INTEGER NOT NULL DEFAULT 0, repository TEXT NOT NULL, install_path TEXT NOT NULL DEFAULT '',
last_seen_at TEXT NOT NULL, UNIQUE(repository, name));
INSERT INTO appimage_packages(name, repository, last_seen_at) VALUES('Existing', 'am-FUSION', 'stamp');`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	migrated, err := openAMFusionDB()
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	var size int64
	if err := migrated.QueryRow(`SELECT size_bytes FROM appimage_packages WHERE name='Existing'`).Scan(&size); err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("legacy row size default=%d, want 0", size)
	}
	if err := migrateAMFusionSizeColumn(migrated); err != nil {
		t.Fatalf("migration must be idempotent: %v", err)
	}
}

func TestAMFusionPlainOutputHasAdditiveFifthSizeColumn(t *testing.T) {
	line := amFusionPlainRecord(amFusionPackage{Name: "Krita", Version: "5.2.0", Type: "appimage", SizeBytes: 262144000})
	fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
	if len(fields) != 5 || strings.Join(fields[:4], "\t") != "Krita\t5.2.0\tappimage\tam-FUSION" || fields[4] != "262144000" {
		t.Fatalf("unexpected AM plain record (legacy 4 columns remain a prefix): %q", line)
	}
}
