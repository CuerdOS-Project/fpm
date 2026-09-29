package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

const defaultFPMDBPath = "/var/lib/fpm/fpm.db"

// fpmDBPath is a package variable so tests can use an isolated temporary DB.
// Production always uses the separate global FPM cache, never XBPS's own DB.
var fpmDBPath = defaultFPMDBPath

type cachedPackage struct {
	Name       string
	Version    string
	Summary    string
	LastSeenAt string
}

func parseInstalledPackageSnapshot(output string) []cachedPackage {
	packages := make([]cachedPackage, 0)
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || !isInstalledListStatus(fields[0]) {
			continue
		}
		name, version := splitNissaPackageVersion(fields[1])
		if name == "" || version == "" || !nissaPackageName.MatchString(name) || seen[name] {
			continue
		}
		seen[name] = true
		summary := ""
		if len(fields) > 2 {
			summary = strings.Join(fields[2:], " ")
		}
		packages = append(packages, cachedPackage{Name: name, Version: version, Summary: summary})
	}
	return packages
}

func isInstalledListStatus(status string) bool {
	if status == "ii" {
		return true
	}
	// XBPS versions that use bracketed status columns mark installed entries
	// with one or more asterisks (for example [*]); [-] is not installed.
	return strings.HasPrefix(status, "[") && strings.Contains(status, "*")
}

func refreshFPMDatabase() (int, error) {
	if os.Geteuid() != 0 {
		return 0, errors.New("refreshing the global FPM database requires root privileges")
	}
	if !cmdExists("xbps-query") {
		return 0, errors.New("xbps-query is not available")
	}
	result := runCmd([]string{"xbps-query", "-l"}, true)
	if result.Code != 0 {
		return 0, fmt.Errorf("xbps-query -l failed with exit code %d", result.Code)
	}
	packages := parseInstalledPackageSnapshot(result.Stdout)
	if len(packages) == 0 {
		return 0, errors.New("no installed XBPS packages could be parsed; refusing to replace the FPM snapshot")
	}
	if err := writeFPMDatabaseSnapshot(packages, time.Now().UTC()); err != nil {
		return 0, err
	}
	return len(packages), nil
}

func writeFPMDatabaseSnapshot(packages []cachedPackage, syncedAt time.Time) error {
	if os.Geteuid() != 0 {
		return errors.New("writing the global FPM database requires root privileges")
	}
	return writePackageSnapshotAt(fpmDBPath, packages, syncedAt, 0)
}

func writePackageSnapshotAt(path string, packages []cachedPackage, syncedAt time.Time, ownerUID int) error {
	if err := prepareFPMDBPath(path, ownerUID); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=DELETE; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;`); err != nil {
		return fmt.Errorf("configure FPM database: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS packages (
		name TEXT PRIMARY KEY NOT NULL,
		version TEXT NOT NULL,
		summary TEXT NOT NULL DEFAULT '',
		last_seen_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create FPM package table: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY NOT NULL,
		value TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create FPM metadata table: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}
	if _, err := tx.Exec(`DELETE FROM packages`); err != nil {
		return rollback(err)
	}
	stamp := syncedAt.UTC().Format(time.RFC3339Nano)
	stmt, err := tx.Prepare(`INSERT INTO packages(name, version, summary, last_seen_at) VALUES(?, ?, ?, ?)`)
	if err != nil {
		return rollback(err)
	}
	for _, pkg := range packages {
		if pkg.Name == "" || pkg.Version == "" || !nissaPackageName.MatchString(pkg.Name) {
			_ = stmt.Close()
			return rollback(fmt.Errorf("invalid package record %q", pkg.Name))
		}
		if _, err := stmt.Exec(pkg.Name, pkg.Version, pkg.Summary, stamp); err != nil {
			_ = stmt.Close()
			return rollback(err)
		}
	}
	if err := stmt.Close(); err != nil {
		return rollback(err)
	}
	for key, value := range map[string]string{
		"schema_version": "1",
		"source":         "xbps-query -l",
		"last_sync":      stamp,
		"package_count":  fmt.Sprintf("%d", len(packages)),
	} {
		if _, err := tx.Exec(`INSERT INTO metadata(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := os.Chmod(path, 0644); err != nil {
		return fmt.Errorf("set FPM database permissions: %w", err)
	}
	return nil
}

func prepareFPMDBPath(path string, ownerUID int) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create FPM database directory: %w", err)
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || dirInfo.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("unsafe FPM database directory permissions: %s", dir)
	}
	if dirStat, ok := dirInfo.Sys().(*syscall.Stat_t); ok && int(dirStat.Uid) != ownerUID {
		return fmt.Errorf("FPM database directory is not owned by uid %d: %s", ownerUID, dir)
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("FPM database path is not a regular file: %s", path)
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != ownerUID {
			return fmt.Errorf("FPM database is not owned by uid %d: %s", ownerUID, path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func loadFPMDatabaseSnapshot() ([]cachedPackage, string, error) {
	return loadFPMDatabaseSnapshotAt(fpmDBPath)
}

func loadFPMDatabaseSnapshotAt(path string) ([]cachedPackage, string, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, "", err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return nil, "", err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name, version, summary, last_seen_at FROM packages ORDER BY name`)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	packages := make([]cachedPackage, 0)
	for rows.Next() {
		var pkg cachedPackage
		if err := rows.Scan(&pkg.Name, &pkg.Version, &pkg.Summary, &pkg.LastSeenAt); err != nil {
			return nil, "", err
		}
		packages = append(packages, pkg)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var lastSync string
	if err := db.QueryRow(`SELECT value FROM metadata WHERE key='last_sync'`).Scan(&lastSync); err != nil {
		return nil, "", err
	}
	return packages, lastSync, nil
}

func packageDatabaseInfo() (count int, lastSync string, size int64, err error) {
	packages, lastSync, err := loadFPMDatabaseSnapshot()
	if err != nil {
		return 0, "", 0, err
	}
	info, err := os.Stat(fpmDBPath)
	if err != nil {
		return 0, "", 0, err
	}
	return len(packages), lastSync, info.Size(), nil
}

func syncFPMDatabaseAfterChange() {
	if os.Geteuid() != 0 {
		return
	}
	if _, err := refreshFPMDatabase(); err != nil {
		fmt.Fprintf(os.Stderr, "FPM: warning: package cache was not refreshed: %v\n", err)
	}
}

func dbCommandNeedsRoot(args []string) bool {
	return len(args) == 0 || args[0] != "list"
}

func listFPMDatabase() error {
	packages, lastSync, err := loadFPMDatabaseSnapshot()
	if err != nil {
		return fmt.Errorf("%s: %w", tr("err_fpm_db_missing"), err)
	}
	fmt.Printf("%s: %s\n", tr("lbl_fpm_database"), fpmDBPath)
	fmt.Printf("%s: %d packages | %s: %s\n", tr("lbl_xbps_snapshot"), len(packages), tr("lbl_last_sync"), lastSync)
	for _, pkg := range packages {
		if pkg.Summary != "" {
			fmt.Printf("%s\t%s\t%s\n", pkg.Name, pkg.Version, pkg.Summary)
		} else {
			fmt.Printf("%s\t%s\n", pkg.Name, pkg.Version)
		}
	}
	return nil
}
