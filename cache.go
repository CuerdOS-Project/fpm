package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// xbpsCommand construye argumentos sin confirmar automáticamente. La opción
// -y solo se añade cuando el usuario solicitó --yes/-y a FPM.
func xbpsCommand(program, options string, packages ...string) []string {
	args := []string{program}
	if options != "" {
		args = append(args, options)
	}
	if yesFlag {
		args = append(args, "-y")
	}
	return append(args, packages...)
}

func directoryStats(path string) (bytes int64, files int, exists bool) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return 0, 0, false
	}
	exists = true
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		files++
		if info, statErr := entry.Info(); statErr == nil {
			bytes += info.Size()
		}
		return nil
	})
	return bytes, files, true
}

func cacheInfo() {
	dHeader("%s", tr("hdr_cache_info"))
	for _, path := range []string{"/var/cache/xbps", "/var/db/xbps"} {
		bytes, files, exists := directoryStats(path)
		if !exists {
			fmt.Printf("  %s: %s\n", path, tr("info_not_available"))
			continue
		}
		kind := tr("lbl_cache")
		if path == "/var/db/xbps" {
			kind = tr("lbl_database")
		}
		fmt.Printf("  %s: %s | %d %s | %s\n", kind, path, files, tr("lbl_files"), formatSize(bytes))
	}
	if cmdExists("xbps-query") {
		r := runCmd([]string{"xbps-query", "-l"}, true)
		count := 0
		for _, line := range splitNonEmpty(r.Stdout) {
			if len(line) > 0 {
				count++
			}
		}
		fmt.Printf("  %s: %d\n", tr("lbl_installed_count"), count)
	}
	if count, lastSync, size, err := packageDatabaseInfo(); err == nil {
		fmt.Printf("  %s: %d packages | %s: %s | %s\n", tr("lbl_fpm_snapshot"), count,
			tr("lbl_last_sync"), lastSync, formatSize(size))
	} else {
		fmt.Printf("  %s: %s (%v)\n", tr("lbl_fpm_snapshot"), tr("info_not_available"), err)
	}
}

func cleanXBPSCache() {
	if !checkXBPS() {
		return
	}
	dHeader("%s", tr("hdr_cleaning_cache"))
	r := runCmd(xbpsCommand("xbps-remove", "-O"), false)
	if r.Code == 0 {
		dOK("%s", tr("ok_cache_cleaned"))
	} else {
		dErr("%s", tr("err_cache_clean_failed"))
	}
}

func repairXBPSDatabase() {
	if !checkXBPS() {
		return
	}
	dHeader("%s", tr("hdr_repairing_db"))
	r := runCmd([]string{"xbps-pkgdb", "-a"}, false)
	if r.Code != 0 {
		dErr("%s", tr("err_db_repair_failed"))
		return
	}
	dOK("%s", tr("ok_db_repaired"))
	syncFPMDatabaseAfterChange()
}

func checkXBPSDatabase() {
	if !checkXBPS() {
		return
	}
	dHeader("%s", tr("hdr_checking_integrity"))
	r := runCmd([]string{"xbps-pkgdb", "-k"}, false)
	if r.Code == 0 {
		dOK("%s", tr("ok_check_done"))
	} else {
		dErr("%s", tr("err_db_check_failed"))
	}
}

func splitNonEmpty(value string) []string {
	result := make([]string, 0)
	for _, line := range stringsSplitLines(value) {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func stringsSplitLines(value string) []string {
	if value == "" {
		return nil
	}
	result := make([]string, 0)
	start := 0
	for i, r := range value {
		if r == '\n' {
			result = append(result, value[start:i])
			start = i + 1
		}
	}
	if start < len(value) {
		result = append(result, value[start:])
	}
	return result
}
