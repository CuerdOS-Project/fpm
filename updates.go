package main

import (
	"bufio"
	"fmt"
	"strings"
)

type xbpsUpdateCandidate struct {
	name       string
	newVersion string
}

// parseXBPSUpdateCandidates accepts the package-version rows printed by
// `xbps-install -un` and the newer name/version column form. It is shared by
// the human CLI and NISSA so both clients interpret XBPS output consistently.
func parseXBPSUpdateCandidates(output string) []xbpsUpdateCandidate {
	var candidates []xbpsUpdateCandidate
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) == 0 || strings.EqualFold(fields[0], "name") || strings.HasPrefix(fields[0], "-") {
			continue
		}

		name, version := splitNissaPackageVersion(fields[0])
		if version == "" && len(fields) >= 2 {
			name, version = fields[0], fields[1]
		}
		if !nissaPackageName.MatchString(name) || !looksLikeVersion(version) || seen[name] {
			continue
		}
		seen[name] = true
		candidates = append(candidates, xbpsUpdateCandidate{name: name, newVersion: version})
	}
	return candidates
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
	for _, candidate := range candidates {
		currentVersion := nissaInstalledVersion(candidate.name)
		if currentVersion == "" {
			currentVersion = "?"
		}
		architecture := nissaInstalledProperty(candidate.name, "architecture")
		fmt.Printf("  %-32s %s → %s", candidate.name, currentVersion, candidate.newVersion)
		if architecture != "" {
			fmt.Printf("  (%s)", architecture)
		}
		fmt.Println()
	}
	fmt.Println()
	fmt.Printf("%s\n", fmt.Sprintf(tr("msg_updates_found"), len(candidates)))
}
