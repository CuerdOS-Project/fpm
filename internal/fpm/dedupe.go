package fpm

import (
	"strings"
)

// Deduplication of XBPS search results. The same package can be listed more
// than once when several repositories (current, multilib, nonfree, custom
// ones such as CuerdOS) provide it, possibly with different versions. One row
// per package name is kept: an installed entry wins, then the repository with
// higher priority, then the entry with more metadata, and finally the one
// listed first (xbps-query lists repositories in configuration order). This
// mirrors backend/dedupe.py in Yelena Software.

// xbpsRepoPriority ranks repositories, best first. Unknown repositories rank
// after the known ones; an empty repository (xbps-query -Rs does not print
// it) ranks first so the listing order decides.
var xbpsRepoPriority = []string{"", "current", "cuerdos", "multilib", "nonfree"}

type xbpsRow struct {
	Name, Version, Summary, Repo string
	Installed                    bool
}

func xbpsRepoRank(repo string) int {
	repo = strings.ToLower(strings.TrimSpace(repo))
	for i, candidate := range xbpsRepoPriority {
		if repo == candidate {
			return i
		}
	}
	return len(xbpsRepoPriority)
}

func xbpsRowRichness(row xbpsRow) int {
	score := 0
	if row.Version != "" {
		score++
	}
	if row.Summary != "" {
		score++
	}
	return score
}

// xbpsRowBetter reports whether a should replace b (a strictly better).
func xbpsRowBetter(a, b xbpsRow) bool {
	if a.Installed != b.Installed {
		return a.Installed
	}
	if ra, rb := xbpsRepoRank(a.Repo), xbpsRepoRank(b.Repo); ra != rb {
		return ra < rb
	}
	return xbpsRowRichness(a) > xbpsRowRichness(b)
}

// dedupeXBPSRows keeps one row per package name, preserving first-seen order.
func dedupeXBPSRows(rows []xbpsRow) []xbpsRow {
	index := make(map[string]int, len(rows))
	out := make([]xbpsRow, 0, len(rows))
	for _, row := range rows {
		if row.Name == "" {
			continue
		}
		if i, ok := index[row.Name]; ok {
			if xbpsRowBetter(row, out[i]) {
				out[i] = row
			}
			continue
		}
		index[row.Name] = len(out)
		out = append(out, row)
	}
	return out
}

// dedupeXBPSItems applies the same rule to TUI items; items from other
// sources pass through untouched. The key is the package name without the
// version suffix, so foo-1.0_1 and foo-1.1_1 count as the same package.
func dedupeXBPSItems(items []TuiItem) []TuiItem {
	index := map[string]int{}
	out := make([]TuiItem, 0, len(items))
	for _, item := range items {
		if item.Source != "XBPS" {
			out = append(out, item)
			continue
		}
		key, _ := splitPlutoPackageVersion(item.Name)
		if i, ok := index[key]; ok {
			if item.Installed && !out[i].Installed {
				out[i] = item
			}
			continue
		}
		index[key] = len(out)
		out = append(out, item)
	}
	return out
}

// parseXBPSSearchRows turns `xbps-query -Rs` output into deduplicated rows.
// installedListing is the output of `xbps-query -l`; when haveInstalled is
// false (that call failed) only the [*] marker is trusted.
func parseXBPSSearchRows(output, installedListing string, haveInstalled bool, limit int) []xbpsRow {
	var rows []xbpsRow
	var installedIndex installedNameIndex
	if haveInstalled {
		installedIndex = newInstalledNameIndex(installedListing)
	}
	// forEachLine has no line-length limit, so rows after an oversized line
	// are never lost (bufio.Scanner stopped silently at 1 MiB).
	forEachLine(output, func(rawLine string) bool {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			return true
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
			return true
		}
		name, versionText := splitPlutoPackageVersion(fields[0])
		if name == "" || !plutoPackageName.MatchString(name) {
			return true
		}
		if !installed && haveInstalled {
			installed = installedIndex.has(name)
		}
		summary := ""
		if len(fields) > 1 {
			summary = strings.Join(fields[1:], " ")
		}
		rows = append(rows, xbpsRow{Name: name, Version: versionText, Summary: summary, Installed: installed})
		return true
	})
	rows = dedupeXBPSRows(rows)
	if limit >= 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}
