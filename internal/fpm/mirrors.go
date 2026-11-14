package fpm

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	voidFallbackMarker = "# Managed by FPM: Void mirror fallback"
	voidFallbackFile   = "/etc/xbps.d/zzzz-fpm-void-fallback.conf"
	voidGlobalRepoHost = "repo-default.voidlinux.org"
)

// Kept as a variable so tests can use a private temporary XBPS configuration.
var voidFallbackConfigPath = voidFallbackFile

var errVoidFallbackUnmanaged = errors.New("fallback configuration is not managed by FPM")

// This is the public mirror list maintained by Void's xmirror project. We use
// it only to recognize an already-configured Void mirror; downloads still use
// the user's configured mirror and the official global endpoint as fallback.
var officialVoidMirrorHosts = map[string]struct{}{
	"repo-default.voidlinux.org":    {},
	"repo-fi.voidlinux.org":         {},
	"repo-de.voidlinux.org":         {},
	"repo-fastly.voidlinux.org":     {},
	"mirrors.summithq.com":          {},
	"mirrors.cicku.me":              {},
	"mirror.nju.edu.cn":             {},
	"mirrors.bfsu.edu.cn":           {},
	"mirrors.tuna.tsinghua.edu.cn":  {},
	"mirror.mmdbalkhi.ir":           {},
	"mirrors.kubarcloud.net":        {},
	"repo.jing.rocks":               {},
	"mirror.freedif.org":            {},
	"mirror.heigou.pe.kr":           {},
	"mirror.meowsmp.net":            {},
	"voidlinux.mirror-services.net": {},
	"ftp.dk.xemacs.org":             {},
	"mirrors.dotsrc.org":            {},
	"void.mirror.lynnfra.net":       {},
	"ftp.cc.uoc.gr":                 {},
	"voidlinux.mirror.garr.it":      {},
	"void.sakamoto.pl":              {},
	"ftp.debian.ru":                 {},
	"mirror.yandex.ru":              {},
	"ftp.lysator.liu.se":            {},
	"mirror.accum.se":               {},
	"mirror.puzzle.ch":              {},
	"mirror.vofr.net":               {},
	"mirrors.lug.mtu.edu":           {},
	"void.tuxmirrors.xyz":           {},
	"mirror.aarnet.edu.au":          {},
	"void.voidbr.org":               {},
	"void.voidlinux.com.br":         {},
	"mirror.linux.ec":               {},
}

var supportedVoidRepoRoutes = map[string]struct{}{
	"current":                  {},
	"current/musl":             {},
	"current/nonfree":          {},
	"current/multilib":         {},
	"current/multilib/nonfree": {},
	"current/debug":            {},
	"current/musl/nonfree":     {},
	"current/aarch64":          {},
}

func parseXBPSRepositoryURLs(output string) []string {
	seen := make(map[string]bool)
	var urls []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		for _, field := range strings.Fields(scanner.Text()) {
			candidate := strings.TrimRight(field, ",;)]}")
			if !strings.HasPrefix(candidate, "https://") && !strings.HasPrefix(candidate, "http://") {
				continue
			}
			parsed, err := url.Parse(candidate)
			if err != nil || parsed.Hostname() == "" || parsed.User != nil {
				continue
			}
			candidate = strings.TrimRight(parsed.String(), "/")
			if !seen[candidate] {
				seen[candidate] = true
				urls = append(urls, candidate)
			}
			break
		}
	}
	return urls
}

// parseOfficialVoidRepo returns the parsed URL and its canonical repository
// route (for example current/multilib). Unknown/custom repositories are never
// rewritten or given an official Void fallback.
func parseOfficialVoidRepo(raw string) (*url.URL, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.Port() != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, "", false
	}
	host := strings.ToLower(parsed.Hostname())
	if _, ok := officialVoidMirrorHosts[host]; !ok {
		return nil, "", false
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, segment := range segments {
		if segment != "current" {
			continue
		}
		route := strings.Join(segments[i:], "/")
		if _, ok := supportedVoidRepoRoutes[route]; !ok {
			return nil, "", false
		}
		return parsed, route, true
	}
	return nil, "", false
}

func voidFallbackURL(repositoryURL string) (string, bool) {
	parsed, route, ok := parseOfficialVoidRepo(repositoryURL)
	if !ok || strings.EqualFold(parsed.Hostname(), voidGlobalRepoHost) {
		return "", false
	}
	return "https://" + voidGlobalRepoHost + "/" + route, true
}

func voidFallbackURLs(repositoryURLs []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, repositoryURL := range repositoryURLs {
		fallback, ok := voidFallbackURL(repositoryURL)
		if !ok || seen[fallback] {
			continue
		}
		seen[fallback] = true
		result = append(result, fallback)
	}
	return result
}

func isOfficialVoidRepoURL(repositoryURL string) bool {
	_, _, ok := parseOfficialVoidRepo(repositoryURL)
	return ok
}

func fallbackConfigContents(repositoryURLs []string) (string, error) {
	seen := make(map[string]bool)
	urls := make([]string, 0, len(repositoryURLs))
	for _, raw := range repositoryURLs {
		parsed, _, ok := parseOfficialVoidRepo(raw)
		if !ok || !strings.EqualFold(parsed.Hostname(), voidGlobalRepoHost) {
			return "", fmt.Errorf("invalid Void fallback repository URL: %s", raw)
		}
		canonical := strings.TrimRight(parsed.String(), "/")
		if !seen[canonical] {
			seen[canonical] = true
			urls = append(urls, canonical)
		}
	}
	if len(urls) == 0 {
		return "", errors.New("no Void fallback URLs")
	}
	sort.Strings(urls)
	var builder strings.Builder
	builder.WriteString(voidFallbackMarker)
	builder.WriteString("\n# Added after existing XBPS repositories; existing priority is preserved.\n")
	for _, repositoryURL := range urls {
		builder.WriteString("repository=")
		builder.WriteString(repositoryURL)
		builder.WriteByte('\n')
	}
	return builder.String(), nil
}

func writeVoidFallbackConfig(path string, repositoryURLs []string) error {
	content, err := fallbackConfigContents(repositoryURLs)
	if err != nil {
		return err
	}
	if existing, readErr := os.ReadFile(path); readErr == nil {
		if !strings.HasPrefix(string(existing), voidFallbackMarker+"\n") {
			return errVoidFallbackUnmanaged
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".fpm-void-fallback-*.conf")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func readVoidFallbackConfig(path string) (bool, []string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if !strings.HasPrefix(string(data), voidFallbackMarker+"\n") {
		return false, nil, errVoidFallbackUnmanaged
	}
	var urls []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "repository=") {
			urls = append(urls, strings.TrimSpace(strings.TrimPrefix(line, "repository=")))
		}
	}
	return true, urls, nil
}

func removeVoidFallbackConfig(path string) (bool, error) {
	managed, _, err := readVoidFallbackConfig(path)
	if err != nil || !managed {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}

func mirrorPlutoRecords(repositoryURLs, fallbackURLs []string) []map[string]any {
	configuredFallbacks := make(map[string]bool, len(fallbackURLs))
	for _, repositoryURL := range fallbackURLs {
		configuredFallbacks[repositoryURL] = true
	}
	seen := make(map[string]bool)
	var records []map[string]any
	for _, repositoryURL := range repositoryURLs {
		if !isOfficialVoidRepoURL(repositoryURL) || seen[repositoryURL] {
			continue
		}
		seen[repositoryURL] = true
		fallbackURL, _ := voidFallbackURL(repositoryURL)
		isFallback := strings.Contains(repositoryURL, "://"+voidGlobalRepoHost+"/") && configuredFallbacks[repositoryURL]
		records = append(records, map[string]any{
			"repository_url":      repositoryURL,
			"fallback_url":        fallbackURL,
			"fallback_configured": fallbackURL != "" && configuredFallbacks[fallbackURL],
			"is_fallback":         isFallback,
			"official_void":       true,
		})
	}
	return records
}

func enableVoidMirrorFallback() {
	if !cmdExists("xbps-query") {
		dErr("%s", tr("repos_query_failed"))
		return
	}
	result := runCmd([]string{"xbps-query", "-L"}, true)
	if result.Code != 0 {
		dErr("%s", tr("repos_query_failed"))
		return
	}
	repositoryURLs := parseXBPSRepositoryURLs(result.Stdout)
	fallbackURLs := voidFallbackURLs(repositoryURLs)
	if len(fallbackURLs) == 0 {
		for _, repositoryURL := range repositoryURLs {
			if parsed, _, ok := parseOfficialVoidRepo(repositoryURL); ok && strings.EqualFold(parsed.Hostname(), voidGlobalRepoHost) {
				dInfo("%s", tr("repos_fallback_already_configured"))
				return
			}
		}
		dErr("%s", tr("repos_fallback_no_void_repositories"))
		return
	}
	if err := writeVoidFallbackConfig(voidFallbackConfigPath, fallbackURLs); err != nil {
		if errors.Is(err, errVoidFallbackUnmanaged) {
			dErr("%s", tr("repos_fallback_unmanaged"))
		} else {
			dErr("%s: %v", tr("repos_fallback_write_failed"), err)
		}
		return
	}
	dOK("%s", tr("repos_fallback_enabled"))
}

func showVoidMirrorFallbackStatus() {
	managed, urls, err := readVoidFallbackConfig(voidFallbackConfigPath)
	if err != nil {
		if errors.Is(err, errVoidFallbackUnmanaged) {
			dErr("%s", tr("repos_fallback_unmanaged"))
		} else {
			dErr("%s: %v", tr("repos_fallback_write_failed"), err)
		}
		return
	}
	if !managed {
		dInfo("%s", tr("repos_fallback_status_off"))
		return
	}
	dHeader(tr("repos_fallback_status_on"), len(urls))
	for _, repositoryURL := range urls {
		fmt.Printf("  %s\n", repositoryURL)
	}
}

func disableVoidMirrorFallback() {
	removed, err := removeVoidFallbackConfig(voidFallbackConfigPath)
	if err != nil {
		if errors.Is(err, errVoidFallbackUnmanaged) {
			dErr("%s", tr("repos_fallback_unmanaged"))
		} else {
			dErr("%s: %v", tr("repos_fallback_disable_failed"), err)
		}
		return
	}
	if !removed {
		dInfo("%s", tr("repos_fallback_status_off"))
		return
	}
	dOK("%s", tr("repos_fallback_disabled"))
}

func plutoMirrors(args []string) int {
	if len(args) != 0 {
		return plutoError(plutoVersion2, "mirrors", "invalid_arguments", 2)
	}
	if !cmdExists("xbps-query") {
		return plutoError(plutoVersion2, "mirrors", "xbps_unavailable", 1)
	}
	result := runCmd([]string{"xbps-query", "-L"}, true)
	if result.Code != 0 {
		return plutoError(plutoVersion2, "mirrors", "repository_query_failed", 1)
	}
	_, configuredFallbacks, configErr := readVoidFallbackConfig(voidFallbackConfigPath)
	if configErr != nil && !errors.Is(configErr, errVoidFallbackUnmanaged) {
		return plutoError(plutoVersion2, "mirrors", "fallback_status_unavailable", 1)
	}
	if errors.Is(configErr, errVoidFallbackUnmanaged) {
		configuredFallbacks = nil
	}
	records := mirrorPlutoRecords(parseXBPSRepositoryURLs(result.Stdout), configuredFallbacks)
	for _, record := range records {
		if err := plutoEmitVersion(plutoVersion2, "mirrors.result", record, nil, ""); err != nil {
			return 1
		}
	}
	fallbackEnabled := len(configuredFallbacks) > 0
	if err := plutoEmitVersion(plutoVersion2, "mirrors.done", map[string]any{
		"ok": true, "count": len(records), "fallback_enabled": fallbackEnabled,
		"fallback_urls": configuredFallbacks,
	}, plutoBool(true), ""); err != nil {
		return 1
	}
	return 0
}
