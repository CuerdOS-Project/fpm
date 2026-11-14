package fpm

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// PLUTO is FPM's local, structured communication protocol for
// sibling desktop applications. v1 remains search-only for compatibility;
// v2 adds read-only inventory, update and package-detail queries. Mutations
// continue to use FPM's ordinary CLI and the caller's trusted authorization UI.
const plutoProtocol = "PLUTO"
const plutoVersion = 1
const plutoVersion2 = 2
const plutoMaxQueryLength = 512

// Sin tope: se emiten todos los resultados encontrados (antes 5000).
const plutoMaxResults = int(^uint(0) >> 1)

var plutoPackageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+._-]*$`)

type plutoSignal struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
	Signal   string `json:"signal"`
	Payload  any    `json:"payload,omitempty"`
	OK       *bool  `json:"ok,omitempty"`
	Error    string `json:"error,omitempty"`
}

func plutoEmitVersion(version int, signal string, payload any, ok *bool, errCode string) error {
	message := plutoSignal{
		Protocol: plutoProtocol,
		Version:  version,
		Signal:   signal,
		Payload:  payload,
		OK:       ok,
		Error:    errCode,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(message)
}

func plutoEmit(signal string, payload any, ok *bool, errCode string) error {
	return plutoEmitVersion(plutoVersion, signal, payload, ok, errCode)
}

func plutoBool(value bool) *bool { return &value }

func plutoError(version int, operation, code string, exitCode int) int {
	signal := operation + ".error"
	if operation == "error" {
		signal = "error"
	}
	_ = plutoEmitVersion(version, signal, nil, plutoBool(false), code)
	return exitCode
}

func plutoSearch(args []string) int {
	return plutoSearchVersion(plutoVersion, args)
}

func plutoSearchVersion(version int, args []string) int {
	if len(args) != 1 {
		return plutoError(version, "search", "invalid_arguments", 2)
	}
	query := strings.TrimSpace(args[0])
	if query == "" || len([]rune(query)) > plutoMaxQueryLength || strings.IndexFunc(query, unicode.IsControl) >= 0 {
		return plutoError(version, "search", "invalid_query", 2)
	}
	if version == plutoVersion {
		return plutoSearchXBPSOnly(query)
	}
	return plutoSearchV2(query)
}

func plutoSearchXBPSOnly(query string) int {
	if !cmdExists("xbps-query") {
		return plutoError(plutoVersion, "search", "xbps_unavailable", 1)
	}
	searchResult := runCmd([]string{"xbps-query", "-Rs", query}, true)
	if searchResult.Code != 0 {
		_ = plutoEmitVersion(plutoVersion, "search.error", map[string]any{"exit_code": searchResult.Code}, plutoBool(false), "repository_search_failed")
		return 1
	}
	installedResult := runCmd([]string{"xbps-query", "-l"}, true)
	count := 0
	for _, row := range parseXBPSSearchRows(searchResult.Stdout, installedResult.Stdout, installedResult.Code == 0, plutoMaxResults) {
		if err := plutoEmitVersion(plutoVersion, "search.result", map[string]any{
			"name": row.Name, "version": row.Version, "summary": row.Summary, "installed": row.Installed,
		}, nil, ""); err != nil {
			return 1
		}
		count++
	}
	if err := plutoEmitVersion(plutoVersion, "search.done", map[string]any{"ok": true, "count": count}, plutoBool(true), ""); err != nil {
		return 1
	}
	return 0
}

func plutoEmitSearchSourceError(manager, code string) error {
	payload := map[string]any{"manager": manager}
	if manager == "am" {
		payload["repository"] = amFusionRepoName
	}
	return plutoEmitVersion(plutoVersion2, "search.source_error", payload, plutoBool(false), code)
}

func plutoSearchV2(query string) int {
	count := 0
	complete := true
	sources := make([]string, 0, 2)
	hasXBPS := cmdExists("xbps-query")
	if hasXBPS {
		searchResult := runCmd([]string{"xbps-query", "-Rs", query}, true)
		if searchResult.Code != 0 {
			complete = false
			_ = plutoEmitSearchSourceError("xbps", "repository_search_failed")
		} else {
			sources = append(sources, "xbps")
			installedResult := runCmd([]string{"xbps-query", "-l"}, true)
			for _, row := range parseXBPSSearchRows(searchResult.Stdout, installedResult.Stdout, installedResult.Code == 0, plutoMaxResults) {
				if err := plutoEmitVersion(plutoVersion2, "search.result", map[string]any{
					"name": row.Name, "version": row.Version, "summary": row.Summary, "installed": row.Installed,
					"manager": "xbps",
				}, nil, ""); err != nil {
					return 1
				}
				count++
			}
		}
	} else {
		complete = false
		_ = plutoEmitSearchSourceError("xbps", "xbps_unavailable")
	}

	var enabled bool
	var enabledErr error
	if amBackendAvailable() || amFusionDatabaseExists() {
		enabled, enabledErr = amFusionBackendEnabled()
	}
	if enabledErr != nil {
		complete = false
		_ = plutoEmitSearchSourceError("am", "repository_state_unavailable")
	} else if enabled {
		if !amBackendAvailable() {
			complete = false
			_ = plutoEmitSearchSourceError("am", "backend_unavailable")
		} else {
			output, err := searchAMFusion(query)
			if err != nil {
				complete = false
				_ = plutoEmitSearchSourceError("am", "repository_search_failed")
			} else {
				sources = append(sources, "am")
				cached, cacheErr := amFusionPackageIndex()
				if cacheErr != nil {
					complete = false
					_ = plutoEmitSearchSourceError("am", "inventory_unavailable")
					cached = nil
				}
				for _, pkg := range parseAMFusionCatalog(output) {
					if count >= plutoMaxResults {
						break
					}
					stored := cached[strings.ToLower(pkg.Name)]
					if err := plutoEmitVersion(plutoVersion2, "search.result", map[string]any{
						"name": pkg.Name, "version": stored.Version, "summary": pkg.Description,
						"installed": stored.Installed, "manager": "am", "repository": amFusionRepoName,
						"size_bytes": stored.SizeBytes, "icon_url": amFusionIconURL(pkg.Name),
					}, nil, ""); err != nil {
						return 1
					}
					count++
				}
			}
		}
	}

	if len(sources) == 0 {
		return plutoError(plutoVersion2, "search", "search_sources_unavailable", 1)
	}
	done := map[string]any{"ok": true, "count": count, "sources": sources, "complete": complete}
	if err := plutoEmitVersion(plutoVersion2, "search.done", done, plutoBool(true), ""); err != nil {
		return 1
	}
	return 0
}

func plutoInstalled(args []string) int {
	if len(args) != 0 {
		return plutoError(plutoVersion2, "installed", "invalid_arguments", 2)
	}
	if !cmdExists("xbps-query") {
		return plutoError(plutoVersion2, "installed", "xbps_unavailable", 1)
	}
	result := runCmd([]string{"xbps-query", "-l"}, true)
	if result.Code != 0 {
		_ = plutoEmitVersion(plutoVersion2, "installed.error", map[string]any{"exit_code": result.Code}, plutoBool(false), "installed_query_failed")
		return 1
	}
	count := 0
	emitFailed := false
	// forEachLine has no line-length limit: one oversized line must not abort
	// the whole query (bufio.Scanner failed at 1 MiB).
	forEachLine(result.Stdout, func(line string) bool {
		if count >= plutoMaxResults {
			return false
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !isInstalledListStatus(fields[0]) {
			return true
		}
		name, versionText := splitPlutoPackageVersion(fields[1])
		if name == "" || !plutoPackageName.MatchString(name) {
			return true
		}
		summary := ""
		if len(fields) > 2 {
			summary = strings.Join(fields[2:], " ")
		}
		if err := plutoEmitVersion(plutoVersion2, "installed.result", map[string]any{
			"name": name, "version": versionText, "summary": summary, "installed": true,
		}, nil, ""); err != nil {
			emitFailed = true
			return false
		}
		count++
		return true
	})
	if emitFailed {
		return 1
	}
	_ = plutoEmitVersion(plutoVersion2, "installed.done", map[string]any{"ok": true, "count": count}, plutoBool(true), "")
	return 0
}

func plutoUpdates(args []string) int {
	if len(args) != 0 {
		return plutoError(plutoVersion2, "updates", "invalid_arguments", 2)
	}
	if !cmdExists("xbps-install") || !cmdExists("xbps-query") {
		return plutoError(plutoVersion2, "updates", "xbps_unavailable", 1)
	}
	result := runCmd([]string{"xbps-install", "-un"}, true)
	if result.Code != 0 {
		_ = plutoEmitVersion(plutoVersion2, "updates.error", map[string]any{"exit_code": result.Code}, plutoBool(false), "update_query_failed")
		return 1
	}
	candidates := parseXBPSUpdateCandidates(result.Stdout)
	details := resolveUpdateDetails(candidates)
	count := 0
	for i, candidate := range candidates {
		if count >= plutoMaxResults {
			break
		}
		name, newVersion := candidate.name, candidate.newVersion
		currentVersion, arch := details[i].current, details[i].arch
		if currentVersion == "" {
			continue
		}
		if err := plutoEmitVersion(plutoVersion2, "updates.result", map[string]any{
			"name": name, "current_version": currentVersion, "new_version": newVersion,
			"arch": arch, "manager": "xbps",
		}, nil, ""); err != nil {
			return 1
		}
		count++
	}
	_ = plutoEmitVersion(plutoVersion2, "updates.done", map[string]any{"ok": true, "count": count}, plutoBool(true), "")
	return 0
}

func plutoInfo(args []string) int {
	if len(args) != 1 || !plutoPackageName.MatchString(args[0]) {
		return plutoError(plutoVersion2, "info", "invalid_arguments", 2)
	}
	name := args[0]
	if !cmdExists("xbps-query") {
		return plutoError(plutoVersion2, "info", "xbps_unavailable", 1)
	}
	result := runCmd([]string{"xbps-query", "-R", name}, true)
	if result.Code != 0 || strings.TrimSpace(result.Stdout) == "" {
		return plutoError(plutoVersion2, "info", "package_not_found", 1)
	}
	properties := make(map[string]string)
	for _, line := range strings.Split(result.Stdout, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			properties[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	pkgver := properties["pkgver"]
	_, versionText := splitPlutoPackageVersion(pkgver)
	if versionText == "" {
		versionText = pkgver
	}
	installedVersion := plutoInstalledVersion(name)
	installed := installedVersion != ""
	var sizeBytes int64
	if raw := properties["installed_size"]; raw != "" {
		_, _ = fmt.Sscan(strings.Fields(raw)[0], &sizeBytes)
	}
	payload := map[string]any{
		"name": name, "version": versionText, "summary": properties["short_desc"],
		"description": properties["long_desc"], "architecture": properties["architecture"],
		"license": properties["license"], "homepage": properties["homepage"],
		"installed": installed, "installed_version": installedVersion,
		"installed_size": sizeBytes,
	}
	_ = plutoEmitVersion(plutoVersion2, "info.result", payload, nil, "")
	_ = plutoEmitVersion(plutoVersion2, "info.done", map[string]any{"ok": true, "count": 1}, plutoBool(true), "")
	return 0
}

func plutoInstalledVersion(name string) string {
	return plutoInstalledProperty(name, "pkgver")
}

func plutoInstalledProperty(name, property string) string {
	result := runCmd([]string{"xbps-query", "-p", property, name}, true)
	if result.Code != 0 {
		return ""
	}
	value := strings.TrimSpace(result.Stdout)
	if property == "pkgver" {
		if _, versionText := splitPlutoPackageVersion(value); versionText != "" {
			return versionText
		}
	}
	return value
}

func looksLikeVersion(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// plutoInstalledPackage is the one-shot lookup. Code that checks many names
// against the same listing must build newInstalledNameIndex once.
func plutoInstalledPackage(name, listing string) bool {
	return newInstalledNameIndex(listing).has(name)
}

func splitPlutoPackageVersion(nameVersion string) (string, string) {
	for i := len(nameVersion) - 1; i > 0; i-- {
		if nameVersion[i] == '-' && i+1 < len(nameVersion) && nameVersion[i+1] >= '0' && nameVersion[i+1] <= '9' {
			return nameVersion[:i], nameVersion[i+1:]
		}
	}
	return nameVersion, ""
}

func runPluto(args []string) int {
	if len(args) < 2 {
		return plutoError(plutoVersion, "error", "unsupported_protocol_version", 2)
	}
	switch args[0] {
	case "v1":
		switch args[1] {
		case "hello":
			if len(args) != 2 {
				return plutoError(plutoVersion, "error", "invalid_arguments", 2)
			}
			_ = plutoEmit("hello", map[string]any{"capabilities": []string{"search"}}, plutoBool(true), "")
			return 0
		case "search":
			return plutoSearch(args[2:])
		default:
			return plutoError(plutoVersion, "error", "unknown_call", 2)
		}
	case "v2":
		switch args[1] {
		case "hello":
			if len(args) != 2 {
				return plutoError(plutoVersion2, "error", "invalid_arguments", 2)
			}
			_ = plutoEmitVersion(plutoVersion2, "hello", map[string]any{
				"capabilities": []string{"search", "search-am", "installed", "updates", "info", "mirrors", "kernels"},
			}, plutoBool(true), "")
			return 0
		case "search":
			return plutoSearchVersion(plutoVersion2, args[2:])
		case "installed":
			return plutoInstalled(args[2:])
		case "updates":
			return plutoUpdates(args[2:])
		case "info":
			return plutoInfo(args[2:])
		case "mirrors":
			return plutoMirrors(args[2:])
		case "kernels":
			return plutoKernels(args[2:])
		default:
			return plutoError(plutoVersion2, "error", "unknown_call", 2)
		}
	default:
		return plutoError(plutoVersion, "error", "unsupported_protocol_version", 2)
	}
}
