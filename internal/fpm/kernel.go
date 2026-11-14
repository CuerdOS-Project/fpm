package fpm

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kernel management for Void Linux. The inventory is read directly from
// /boot, /usr/lib/modules and XBPS, so vkpurge(8) is not required. A kernel
// owned by an installed linuxX.Y package is removed with xbps-remove; a
// leftover from an older version of a package is purged with vkpurge when it
// is installed, or natively otherwise. The running kernel is never removed.

var (
	kernelBootDir    = "/boot"
	kernelModulesDir = "/usr/lib/modules"
	kernelHooksDir   = "/etc/kernel.d"
)

var (
	kernelVersionPattern = regexp.MustCompile(`^[0-9][0-9A-Za-z._+~-]*$`)
	kernelPackagePattern = regexp.MustCompile(`^linux[0-9]+\.[0-9]+$`)
	kernelNumberPattern  = regexp.MustCompile(`[0-9]+|[^0-9]+`)
)

type kernelEntry struct {
	Version string
	Running bool
	Package string
}

func (k kernelEntry) removable() bool { return !k.Running }

// kernelCommandNeedsRoot reports whether a `fpm kernel ...` invocation
// changes the system. Listing never needs privileges.
func kernelCommandNeedsRoot(args []string) bool {
	return len(args) > 0 && isKernelRemoveWord(args[0])
}

func isKernelRemoveWord(word string) bool {
	switch strings.ToLower(word) {
	case "rm", "remove", "purge":
		return true
	}
	return false
}

// validKernelTarget accepts the literal "all" or a kernel version such as
// 6.6.30_1; anything that could be parsed as an option or path is rejected.
func validKernelTarget(target string) bool {
	return target == "all" || (len(target) <= 128 && kernelVersionPattern.MatchString(target))
}

func kernelVersionLess(a, b string) bool {
	left, right := kernelNumberPattern.FindAllString(a, -1), kernelNumberPattern.FindAllString(b, -1)
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] == right[i] {
			continue
		}
		x, errX := strconv.Atoi(left[i])
		y, errY := strconv.Atoi(right[i])
		if errX == nil && errY == nil {
			return x < y
		}
		return left[i] < right[i]
	}
	return len(left) < len(right)
}

// kernelPackageName maps 6.6.30_1 to the conventional Void package linux6.6.
func kernelPackageName(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	return "linux" + parts[0] + "." + parts[1]
}

// runningKernelVersion returns `uname -r`; Void reports it in the same
// "<version>_<revision>" form that XBPS uses for the kernel packages.
func runningKernelVersion() string {
	return strings.TrimSpace(runCmd([]string{"uname", "-r"}, true).Stdout)
}

// installedKernelPackages maps each kernel version owned by an installed
// linuxX.Y package to that package's name.
func installedKernelPackages() map[string]string {
	owners := map[string]string{}
	for _, line := range strings.Split(completionQuiet("xbps-query", "-l"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		cut := strings.LastIndex(fields[1], "-")
		if cut <= 0 {
			continue
		}
		if name, version := fields[1][:cut], fields[1][cut+1:]; kernelPackagePattern.MatchString(name) {
			owners[version] = name
		}
	}
	return owners
}

func kernelVersionsOnDisk() ([]string, bool) {
	seen := map[string]bool{}
	readable := false
	if items, err := os.ReadDir(kernelBootDir); err == nil {
		readable = true
		for _, item := range items {
			if version, ok := strings.CutPrefix(item.Name(), "vmlinuz-"); ok && kernelVersionPattern.MatchString(version) {
				seen[version] = true
			}
		}
	}
	if items, err := os.ReadDir(kernelModulesDir); err == nil {
		readable = true
		for _, item := range items {
			if item.IsDir() && kernelVersionPattern.MatchString(item.Name()) {
				seen[item.Name()] = true
			}
		}
	}
	versions := make([]string, 0, len(seen))
	for version := range seen {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return kernelVersionLess(versions[i], versions[j]) })
	return versions, readable
}

// listKernelEntries returns every kernel found on disk, oldest first.
func listKernelEntries() ([]kernelEntry, bool) {
	versions, ok := kernelVersionsOnDisk()
	if !ok {
		return nil, false
	}
	running := runningKernelVersion()
	owners := installedKernelPackages()
	entries := make([]kernelEntry, 0, len(versions))
	for _, version := range versions {
		entries = append(entries, kernelEntry{Version: version, Running: version == running, Package: owners[version]})
	}
	return entries, true
}

func listKernels() {
	running := runningKernelVersion()
	entries, ok := listKernelEntries()
	if !ok {
		dErr("%s", tr("err_kernel_list_failed"))
		return
	}
	dHeader("%s", tr("hdr_kernels"))
	if running != "" {
		fmt.Printf("  %s%s%s  %s\n", green, running, reset, tr("lbl_kernel_running"))
	}
	leftovers := 0
	for _, entry := range entries {
		switch {
		case entry.Running:
		case entry.Package != "":
			fmt.Printf("  %s  %s\n", entry.Version, fmt.Sprintf(tr("lbl_kernel_package"), entry.Package))
		default:
			fmt.Printf("  %s  %s\n", entry.Version, tr("lbl_kernel_removable"))
			leftovers++
		}
	}
	if leftovers == 0 {
		dInfo("%s", tr("info_no_old_kernels"))
	}
}

func runKernelHooks(stage, pkg, version string) {
	dir := filepath.Join(kernelHooksDir, stage)
	items, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, item := range items {
		path := filepath.Join(dir, item.Name())
		if info, err := os.Stat(path); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		if runCmd([]string{path, pkg, version}, false).Code != 0 {
			dWarn(tr("warn_kernel_hook_failed"), item.Name())
		}
	}
}

// purgeKernelFiles removes the files vkpurge would, running the same
// /etc/kernel.d hooks so initramfs and bootloader entries stay consistent.
func purgeKernelFiles(version string) bool {
	pkg := kernelPackageName(version)
	runKernelHooks("pre-remove", pkg, version)
	ok := true
	for _, prefix := range []string{"config", "initramfs", "System.map", "vmlinux", "vmlinuz"} {
		for _, suffix := range []string{"", ".img"} {
			path := filepath.Join(kernelBootDir, prefix+"-"+version+suffix)
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				ok = false
			}
		}
	}
	if err := os.RemoveAll(filepath.Join(kernelModulesDir, version)); err != nil {
		ok = false
	}
	runKernelHooks("post-remove", pkg, version)
	return ok
}

func removeKernelEntry(entry kernelEntry) bool {
	switch {
	case entry.Package != "":
		return runCmd([]string{"xbps-remove", "-y", entry.Package}, false).Code == 0
	case cmdExists("vkpurge"):
		return runCmd([]string{"vkpurge", "rm", entry.Version}, false).Code == 0
	}
	return purgeKernelFiles(entry.Version)
}

func removeKernels(targets []string) {
	if len(targets) == 0 {
		dErr("%s", tr("err_missing_kernel"))
		return
	}
	for _, target := range targets {
		if !validKernelTarget(target) {
			dErr(tr("err_invalid_kernel"), target)
			return
		}
		if target == "all" && len(targets) > 1 {
			dErr("%s", tr("err_kernel_all_alone"))
			return
		}
	}
	entries, ok := listKernelEntries()
	if !ok {
		dErr("%s", tr("err_kernel_list_failed"))
		return
	}
	var selected []kernelEntry
	if targets[0] == "all" {
		for _, entry := range entries {
			if entry.removable() && entry.Package == "" {
				selected = append(selected, entry)
			}
		}
		if len(selected) == 0 {
			dInfo("%s", tr("info_no_old_kernels"))
			return
		}
	} else {
		chosen := map[string]bool{}
		for _, target := range targets {
			index := -1
			for i, entry := range entries {
				if entry.Version == target {
					index = i
				}
			}
			switch {
			case index < 0:
				dErr(tr("err_kernel_not_found"), target)
				return
			case !entries[index].removable():
				dErr(tr("err_kernel_running"), target)
				return
			case !chosen[target]:
				chosen[target] = true
				selected = append(selected, entries[index])
			}
		}
	}
	versions := make([]string, len(selected))
	for i, entry := range selected {
		versions[i] = entry.Version
	}
	if !askConfirm(fmt.Sprintf(tr("ask_remove_kernels"), joinArgs(versions))) {
		dInfo("%s", tr("info_remove_cancelled"))
		return
	}
	dHeader("%s: %s", tr("hdr_removing_kernels"), joinArgs(versions))
	for _, entry := range selected {
		if !removeKernelEntry(entry) {
			dErr(tr("err_kernel_remove_failed"), entry.Version)
			return
		}
	}
	dOK("%s", tr("ok_kernels_removed"))
}

func runKernelCommand(args []string) {
	sub := "list"
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}
	switch {
	case sub == "list" || sub == "ls":
		listKernels()
	case sub == "current" || sub == "running":
		if running := runningKernelVersion(); running != "" {
			fmt.Println(running)
		} else {
			dErr("%s", tr("err_kernel_list_failed"))
		}
	case isKernelRemoveWord(sub):
		removeKernels(args[1:])
	default:
		dErr(tr("err_unknown_command"), "kernel "+args[0])
	}
}

// plutoKernels is the read-only PLUTO v2 `kernels` call. Mutations stay in
// `fpm kernel rm`, run through the caller's trusted authorization UI.
func plutoKernels(args []string) int {
	if len(args) != 0 {
		return plutoError(plutoVersion2, "kernels", "invalid_arguments", 2)
	}
	entries, ok := listKernelEntries()
	if !ok {
		return plutoError(plutoVersion2, "kernels", "kernel_query_failed", 1)
	}
	running := runningKernelVersion()
	for _, entry := range entries {
		if err := plutoEmitVersion(plutoVersion2, "kernels.result", map[string]any{
			"version": entry.Version, "running": entry.Running, "removable": entry.removable(),
			"package": entry.Package, "leftover": entry.Package == "",
		}, nil, ""); err != nil {
			return 1
		}
	}
	_ = plutoEmitVersion(plutoVersion2, "kernels.done", map[string]any{
		"ok": true, "count": len(entries), "running": running,
	}, plutoBool(true), "")
	return 0
}
