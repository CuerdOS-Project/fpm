package fpm

import (
	"fmt"
	"strings"
	"sync"
)

type xbpsUpdateCandidate struct {
	name       string
	newVersion string
}

// parseXBPSUpdateCandidates accepts the package-version rows printed by
// `xbps-install -un` and the newer name/version column form. It is shared by
// the human CLI and PLUTO so both clients interpret XBPS output consistently.
func parseXBPSUpdateCandidates(output string) []xbpsUpdateCandidate {
	var candidates []xbpsUpdateCandidate
	seen := make(map[string]bool)
	// forEachLine has no line-length limit: updates listed after an oversized
	// line were silently dropped when this used bufio.Scanner (1 MiB cap).
	forEachLine(output, func(line string) bool {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.EqualFold(fields[0], "name") || strings.HasPrefix(fields[0], "-") {
			return true
		}

		name, version := splitPlutoPackageVersion(fields[0])
		if version == "" && len(fields) >= 2 {
			name, version = fields[0], fields[1]
		}
		if !plutoPackageName.MatchString(name) || !looksLikeVersion(version) || seen[name] {
			return true
		}
		seen[name] = true
		candidates = append(candidates, xbpsUpdateCandidate{name: name, newVersion: version})
		return true
	})
	return candidates
}

// updateDetail is what FPM reports about a pending update besides the new
// version: the installed version and architecture.
type updateDetail struct {
	current string
	arch    string
}

// updateQueryWorkers bounds the number of concurrent xbps-query processes.
const updateQueryWorkers = 8

// installedVersionMap reads every installed package version with a single
// `xbps-query -l` instead of one process per package.
func installedVersionMap() map[string]string {
	versions := make(map[string]string)
	result := runCmd([]string{"xbps-query", "-l"}, true)
	if result.Code != 0 {
		return versions
	}
	forEachLine(result.Stdout, func(line string) bool {
		fields := strings.Fields(line)
		if len(fields) < 2 || !isInstalledListStatus(fields[0]) {
			return true
		}
		name, version := splitPlutoPackageVersion(fields[1])
		if name == "" || version == "" {
			return true
		}
		if _, seen := versions[name]; !seen {
			versions[name] = version
		}
		return true
	})
	return versions
}

// resolveUpdateDetails looks up the installed version and architecture of
// every candidate and returns them in candidate order. Versions come from one
// listing (a per-package `xbps-query -p pkgver` is only the fallback for a
// package missing from it); the architecture still needs `xbps-query -p`, so
// those lookups run on a small worker pool. Thousands of pending updates
// (a system updated once in a long while) then take seconds, not minutes.
// There is intentionally no timeout: package tools may be slow and FPM must
// keep waiting.
func resolveUpdateDetails(candidates []xbpsUpdateCandidate) []updateDetail {
	details := make([]updateDetail, len(candidates))
	if len(candidates) == 0 {
		return details
	}
	versions := installedVersionMap()
	workers := updateQueryWorkers
	if len(candidates) < workers {
		workers = len(candidates)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				name := candidates[i].name
				current := versions[name]
				if current == "" {
					current = plutoInstalledVersion(name)
				}
				details[i] = updateDetail{current: current, arch: plutoInstalledProperty(name, "architecture")}
			}
		}()
	}
	for i := range candidates {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return details
}

// listPendingXBPSUpdates only reads the already-synchronized repository index.
// Installation remains an explicit separate `fpm upgrade` operation.
func listPendingXBPSUpdates() {
	if !checkXBPS() {
		return
	}
	if !cmdExists("xbps-query") {
		dErr("%s", tr("err_updates_query_failed"))
		return
	}

	result := runCmd([]string{"xbps-install", "-un"}, true)
	if result.Code != 0 {
		dErr("%s", tr("err_updates_query_failed"))
		return
	}
	candidates := parseXBPSUpdateCandidates(result.Stdout)
	if len(candidates) == 0 {
		dOK("%s", tr("msg_updates_none"))
		return
	}

	dHeader("%s", tr("hdr_updates_available"))
	details := resolveUpdateDetails(candidates)
	for i, candidate := range candidates {
		currentVersion := details[i].current
		if currentVersion == "" {
			currentVersion = "?"
		}
		architecture := details[i].arch
		fmt.Printf("  %-32s %s → %s", candidate.name, currentVersion, candidate.newVersion)
		if architecture != "" {
			fmt.Printf("  (%s)", architecture)
		}
		fmt.Println()
	}
	fmt.Println()
	fmt.Printf("%s\n", fmt.Sprintf(tr("msg_updates_found"), len(candidates)))
}
