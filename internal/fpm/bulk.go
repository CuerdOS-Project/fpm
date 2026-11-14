package fpm

import "strings"

// Helpers for processing large inputs (tens or hundreds of thousands of
// packages) without quadratic work, per-line limits or per-item allocations.

// forEachLine calls fn for every line of text, without its trailing "\n" or
// "\r\n", and stops early when fn returns false. Unlike bufio.Scanner it has no
// maximum line length: a single huge line can never silently truncate the
// rest of a package listing.
func forEachLine(text string, fn func(line string) bool) {
	for len(text) > 0 {
		line, rest := text, ""
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			line, rest = text[:i], text[i+1:]
		}
		text = rest
		if !fn(strings.TrimSuffix(line, "\r")) {
			return
		}
	}
}

// installedLineIndex answers packageInInstalledList in O(1): it holds the
// first and second column of every line of a listing, which is what that
// function compares against. Build it once per listing, not once per package.
type installedLineIndex map[string]struct{}

func newInstalledLineIndex(listing string) installedLineIndex {
	index := make(installedLineIndex, strings.Count(listing, "\n")+1)
	forEachLine(listing, func(line string) bool {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			index[fields[0]] = struct{}{}
		}
		if len(fields) > 1 {
			index[fields[1]] = struct{}{}
		}
		return true
	})
	return index
}

func (index installedLineIndex) has(pkg string) bool {
	_, ok := index[pkg]
	return ok
}

// installedNameIndex answers plutoInstalledPackage in O(1): for every listed
// package it holds both the full pkgver and the bare package name.
type installedNameIndex map[string]struct{}

func newInstalledNameIndex(listing string) installedNameIndex {
	index := make(installedNameIndex, 2*(strings.Count(listing, "\n")+1))
	forEachLine(listing, func(line string) bool {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return true
		}
		candidate := fields[0]
		if len(fields) > 1 && isInstalledListStatus(fields[0]) {
			candidate = fields[1]
		}
		name, _ := splitPlutoPackageVersion(candidate)
		index[candidate] = struct{}{}
		index[name] = struct{}{}
		return true
	})
	return index
}

func (index installedNameIndex) has(name string) bool {
	_, ok := index[name]
	return ok
}

// containsFold reports whether s contains needle ignoring case. For ASCII
// text (package names and summaries) it compares byte by byte without
// allocating, so filtering a huge list on every keystroke does not create
// garbage per item; other text falls back to strings.ToLower.
func containsFold(s, needle string) bool {
	if needle == "" {
		return true
	}
	if !isASCII(needle) || !isASCII(s) {
		return strings.Contains(strings.ToLower(s), strings.ToLower(needle))
	}
	n := len(needle)
	if len(s) < n {
		return false
	}
	first := lowerASCII(needle[0])
	for i := 0; i+n <= len(s); i++ {
		if lowerASCII(s[i]) != first {
			continue
		}
		j := 1
		for ; j < n; j++ {
			if lowerASCII(s[i+j]) != lowerASCII(needle[j]) {
				break
			}
		}
		if j == n {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func lowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
