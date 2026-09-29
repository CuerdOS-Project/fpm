package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// NISSA (Native Inter-Software Signal API) is FPM's local, structured API for
// sibling desktop applications. v1 remains search-only for compatibility;
// v2 adds read-only inventory, update and package-detail queries. Mutations
// continue to use FPM's ordinary CLI and the caller's trusted authorization UI.
const nissaProtocol = "NISSA"
const nissaVersion = 1
const nissaVersion2 = 2
const nissaMaxQueryLength = 512
const nissaMaxResults = 5000

var nissaPackageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+._-]*$`)

type nissaSignal struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
	Signal   string `json:"signal"`
	Payload  any    `json:"payload,omitempty"`
	OK       *bool  `json:"ok,omitempty"`
	Error    string `json:"error,omitempty"`
}

func nissaEmitVersion(version int, signal string, payload any, ok *bool, errCode string) error {
	message := nissaSignal{
		Protocol: nissaProtocol,
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

func nissaEmit(signal string, payload any, ok *bool, errCode string) error {
	return nissaEmitVersion(nissaVersion, signal, payload, ok, errCode)
}

func nissaBool(value bool) *bool { return &value }

func nissaError(version int, operation, code string, exitCode int) int {
	signal := operation + ".error"
	if operation == "error" {
		signal = "error"
	}
	_ = nissaEmitVersion(version, signal, nil, nissaBool(false), code)
	return exitCode
}

func nissaSearch(args []string) int {
	return nissaSearchVersion(nissaVersion, args)
}

func nissaSearchVersion(version int, args []string) int {
	if len(args) != 1 {
		return nissaError(version, "search", "invalid_arguments", 2)
	}
	query := strings.TrimSpace(args[0])
	if query == "" || len([]rune(query)) > nissaMaxQueryLength || strings.IndexFunc(query, unicode.IsControl) >= 0 {
		return nissaError(version, "search", "invalid_query", 2)
	}
	if !cmdExists("xbps-query") {
		return nissaError(version, "search", "xbps_unavailable", 1)
	}

	searchResult := runCmd([]string{"xbps-query", "-Rs", query}, true)
	if searchResult.Code != 0 {
		_ = nissaEmitVersion(version, "search.error", map[string]any{"exit_code": searchResult.Code}, nissaBool(false), "repository_search_failed")
		return 1
	}
	installedResult := runCmd([]string{"xbps-query", "-l"}, true)

	count := 0
	scanner := bufio.NewScanner(strings.NewReader(searchResult.Stdout))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		if count >= nissaMaxResults {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		installed := false
		if strings.HasPrefix(line, "[") {
			if close := strings.IndexByte(line, ']'); close >= 0 {
				installed = strings.TrimSpace(line[1:close]) == "*"
				line = strings.TrimSpace(line[close+1:])
			}
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name, versionText := splitNissaPackageVersion(fields[0])
		if name == "" || !nissaPackageName.MatchString(name) {
			continue
		}
		if !installed && installedResult.Code == 0 {
			installed = nissaInstalledPackage(name, installedResult.Stdout)
		}
		summary := ""
		if len(fields) > 1 {
			summary = strings.Join(fields[1:], " ")
		}
		if err := nissaEmitVersion(version, "search.result", map[string]any{
			"name": name, "version": versionText, "summary": summary, "installed": installed,
		}, nil, ""); err != nil {
			return 1
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		_ = nissaEmitVersion(version, "search.error", map[string]any{"message": err.Error()}, nissaBool(false), "response_read_failed")
		return 1
	}
	if err := nissaEmitVersion(version, "search.done", map[string]any{"ok": true, "count": count}, nissaBool(true), ""); err != nil {
		return 1
	}
	return 0
}

func nissaInstalled(args []string) int {
	if len(args) != 0 {
		return nissaError(nissaVersion2, "installed", "invalid_arguments", 2)
	}
	if !cmdExists("xbps-query") {
		return nissaError(nissaVersion2, "installed", "xbps_unavailable", 1)
	}
	result := runCmd([]string{"xbps-query", "-l"}, true)
	if result.Code != 0 {
		_ = nissaEmitVersion(nissaVersion2, "installed.error", map[string]any{"exit_code": result.Code}, nissaBool(false), "installed_query_failed")
		return 1
	}
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(result.Stdout))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		if count >= nissaMaxResults {
			break
		}
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) < 2 || !isInstalledListStatus(fields[0]) {
			continue
		}
		name, versionText := splitNissaPackageVersion(fields[1])
		if name == "" || !nissaPackageName.MatchString(name) {
			continue
		}
		summary := ""
		if len(fields) > 2 {
			summary = strings.Join(fields[2:], " ")
		}
		if err := nissaEmitVersion(nissaVersion2, "installed.result", map[string]any{
			"name": name, "version": versionText, "summary": summary, "installed": true,
		}, nil, ""); err != nil {
			return 1
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return nissaError(nissaVersion2, "installed", "response_read_failed", 1)
	}
	_ = nissaEmitVersion(nissaVersion2, "installed.done", map[string]any{"ok": true, "count": count}, nissaBool(true), "")
	return 0
}

func nissaUpdates(args []string) int {
	if len(args) != 0 {
		return nissaError(nissaVersion2, "updates", "invalid_arguments", 2)
	}
	if !cmdExists("xbps-install") || !cmdExists("xbps-query") {
		return nissaError(nissaVersion2, "updates", "xbps_unavailable", 1)
	}
	result := runCmd([]string{"xbps-install", "-un"}, true)
	if result.Code != 0 {
		_ = nissaEmitVersion(nissaVersion2, "updates.error", map[string]any{"exit_code": result.Code}, nissaBool(false), "update_query_failed")
		return 1
	}
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(result.Stdout))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		if count >= nissaMaxResults {
			break
		}
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) == 0 || fields[0] == "Name" || strings.HasPrefix(fields[0], "-") {
			continue
		}
		name, newVersion := splitNissaPackageVersion(fields[0])
		if newVersion == "" {
			// Newer XBPS versions may use columns: name, version, architecture.
			if len(fields) < 2 {
				continue
			}
			name, newVersion = fields[0], fields[1]
		}
		if name == "" || !nissaPackageName.MatchString(name) || !looksLikeVersion(newVersion) {
			continue
		}
		currentVersion := nissaInstalledVersion(name)
		arch := nissaInstalledProperty(name, "architecture")
		if currentVersion == "" {
			continue
		}
		if err := nissaEmitVersion(nissaVersion2, "updates.result", map[string]any{
			"name": name, "current_version": currentVersion, "new_version": newVersion,
			"arch": arch, "manager": "xbps",
		}, nil, ""); err != nil {
			return 1
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return nissaError(nissaVersion2, "updates", "response_read_failed", 1)
	}
	_ = nissaEmitVersion(nissaVersion2, "updates.done", map[string]any{"ok": true, "count": count}, nissaBool(true), "")
	return 0
}

func nissaInfo(args []string) int {
	if len(args) != 1 || !nissaPackageName.MatchString(args[0]) {
		return nissaError(nissaVersion2, "info", "invalid_arguments", 2)
	}
	name := args[0]
	if !cmdExists("xbps-query") {
		return nissaError(nissaVersion2, "info", "xbps_unavailable", 1)
	}
	result := runCmd([]string{"xbps-query", "-R", name}, true)
	if result.Code != 0 || strings.TrimSpace(result.Stdout) == "" {
		return nissaError(nissaVersion2, "info", "package_not_found", 1)
	}
	properties := make(map[string]string)
	for _, line := range strings.Split(result.Stdout, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			properties[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	pkgver := properties["pkgver"]
	_, versionText := splitNissaPackageVersion(pkgver)
	if versionText == "" {
		versionText = pkgver
	}
	installedVersion := nissaInstalledVersion(name)
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
	_ = nissaEmitVersion(nissaVersion2, "info.result", payload, nil, "")
	_ = nissaEmitVersion(nissaVersion2, "info.done", map[string]any{"ok": true, "count": 1}, nissaBool(true), "")
	return 0
}

func nissaInstalledVersion(name string) string {
	return nissaInstalledProperty(name, "pkgver")
}

func nissaInstalledProperty(name, property string) string {
	result := runCmd([]string{"xbps-query", "-p", property, name}, true)
	if result.Code != 0 {
		return ""
	}
	value := strings.TrimSpace(result.Stdout)
	if property == "pkgver" {
		if _, versionText := splitNissaPackageVersion(value); versionText != "" {
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

func nissaInstalledPackage(name, listing string) bool {
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		candidate := fields[0]
		if len(fields) > 1 && isInstalledListStatus(fields[0]) {
			candidate = fields[1]
		}
		candidateName, _ := splitNissaPackageVersion(candidate)
		if candidateName == name || candidate == name {
			return true
		}
	}
	return false
}

func splitNissaPackageVersion(nameVersion string) (string, string) {
	for i := len(nameVersion) - 1; i > 0; i-- {
		if nameVersion[i] == '-' && i+1 < len(nameVersion) && nameVersion[i+1] >= '0' && nameVersion[i+1] <= '9' {
			return nameVersion[:i], nameVersion[i+1:]
		}
	}
	return nameVersion, ""
}

func runNissa(args []string) int {
	if len(args) < 2 {
		return nissaError(nissaVersion, "error", "unsupported_protocol_version", 2)
	}
	switch args[0] {
	case "v1":
		switch args[1] {
		case "hello":
			if len(args) != 2 {
				return nissaError(nissaVersion, "error", "invalid_arguments", 2)
			}
			_ = nissaEmit("hello", map[string]any{"capabilities": []string{"search"}}, nissaBool(true), "")
			return 0
		case "search":
			return nissaSearch(args[2:])
		default:
			return nissaError(nissaVersion, "error", "unknown_call", 2)
		}
	case "v2":
		switch args[1] {
		case "hello":
			if len(args) != 2 {
				return nissaError(nissaVersion2, "error", "invalid_arguments", 2)
			}
			_ = nissaEmitVersion(nissaVersion2, "hello", map[string]any{
				"capabilities": []string{"search", "installed", "updates", "info"},
			}, nissaBool(true), "")
			return 0
		case "search":
			return nissaSearchVersion(nissaVersion2, args[2:])
		case "installed":
			return nissaInstalled(args[2:])
		case "updates":
			return nissaUpdates(args[2:])
		case "info":
			return nissaInfo(args[2:])
		default:
			return nissaError(nissaVersion2, "error", "unknown_call", 2)
		}
	default:
		return nissaError(nissaVersion, "error", "unsupported_protocol_version", 2)
	}
}
