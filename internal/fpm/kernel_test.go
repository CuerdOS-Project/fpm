package fpm

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestValidKernelTarget(t *testing.T) {
	for _, ok := range []string{"all", "6.6.30_1", "5.15.150_2", "6.12.0-rc1_1"} {
		if !validKernelTarget(ok) {
			t.Errorf("expected %q to be accepted", ok)
		}
	}
	for _, bad := range []string{"", "-f", "../x", "6.6;rm", "linux6.6", "ALL", "6 6"} {
		if validKernelTarget(bad) {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}

func TestKernelCommandNeedsRoot(t *testing.T) {
	for args, want := range map[string]bool{"": false, "list": false, "current": false, "rm": true, "remove": true, "purge": true} {
		var in []string
		if args != "" {
			in = []string{args}
		}
		if got := kernelCommandNeedsRoot(in); got != want {
			t.Errorf("kernelCommandNeedsRoot(%v) = %v, want %v", in, got, want)
		}
	}
	if canonicalCommand("kernels") != "kernel" {
		t.Error("kernels should alias kernel")
	}
}

func fakeExecutable(t *testing.T, dir, name, script string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestChooseElevatorPrefersYelenaSudoShim(t *testing.T) {
	root := t.TempDir()
	shimDir := filepath.Join(root, "yelena-aimg-abc123")
	binDir := filepath.Join(root, "bin")
	shim := fakeExecutable(t, shimDir, "sudo", "exit 0")
	fakeExecutable(t, binDir, "doas", "exit 0")
	fakeExecutable(t, binDir, "sudo", "exit 0")
	if !isYelenaSudoShim(shim) || isYelenaSudoShim(filepath.Join(binDir, "sudo")) {
		t.Fatal("shim detection is wrong")
	}
	t.Setenv("FPM_ELEVATOR", "")
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+binDir)
	if got := chooseElevator(); got != "sudo" {
		t.Fatalf("with the Yelena shim first, chooseElevator = %q, want sudo", got)
	}
	t.Setenv("PATH", binDir)
	if got := chooseElevator(); got != "doas" {
		t.Fatalf("without the shim, doas is preferred, got %q", got)
	}
	t.Setenv("FPM_ELEVATOR", "sudo")
	if got := chooseElevator(); got != "sudo" {
		t.Fatalf("FPM_ELEVATOR=sudo ignored, got %q", got)
	}
}

func captureStdout(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	code := fn()
	_ = writer.Close()
	os.Stdout = previous
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(data), code
}

func fakeKernelTree(t *testing.T, versions ...string) string {
	t.Helper()
	root := t.TempDir()
	boot, modules, hooks := filepath.Join(root, "boot"), filepath.Join(root, "modules"), filepath.Join(root, "kernel.d")
	for _, dir := range []string{boot, modules, hooks} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, version := range versions {
		for _, name := range []string{"vmlinuz-" + version, "initramfs-" + version + ".img", "config-" + version} {
			if err := os.WriteFile(filepath.Join(boot, name), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(modules, version), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldBoot, oldModules, oldHooks := kernelBootDir, kernelModulesDir, kernelHooksDir
	kernelBootDir, kernelModulesDir, kernelHooksDir = boot, modules, hooks
	t.Cleanup(func() { kernelBootDir, kernelModulesDir, kernelHooksDir = oldBoot, oldModules, oldHooks })
	return root
}

func fakeKernelTools(t *testing.T, running string) string {
	t.Helper()
	dir := t.TempDir()
	fakeExecutable(t, dir, "uname", "echo "+running)
	fakeExecutable(t, dir, "xbps-query", `[ "$1" = -l ] && printf 'ii linux6.6-6.6.30_1 Linux kernel\nii linux6.12-6.12.1_1 Linux kernel\nii linux-6.12.1_1 meta\nii linux6.6-headers-6.6.30_1 headers\nii linux-firmware-20240101_1 fw\n'`)
	fakeExecutable(t, dir, "xbps-remove", `echo "$@" >> "$0.log"`)
	t.Setenv("PATH", dir)
	return dir
}

func TestKernelVersionLess(t *testing.T) {
	versions := []string{"6.12.1_1", "6.6.30_2", "6.6.30_1", "5.15.150_1"}
	sort.Slice(versions, func(i, j int) bool { return kernelVersionLess(versions[i], versions[j]) })
	want := []string{"5.15.150_1", "6.6.30_1", "6.6.30_2", "6.12.1_1"}
	if !reflect.DeepEqual(versions, want) {
		t.Fatalf("sorted = %v, want %v", versions, want)
	}
	if kernelPackageName("6.12.1_1") != "linux6.12" || kernelPackageName("x") != "" {
		t.Fatal("kernelPackageName is wrong")
	}
}

func TestListKernelEntriesWithoutVkpurge(t *testing.T) {
	fakeKernelTree(t, "6.1.90_1", "6.6.30_1", "6.12.1_1")
	fakeKernelTools(t, "6.12.1_1")
	entries, ok := listKernelEntries()
	if !ok {
		t.Fatal("inventory must not need vkpurge")
	}
	want := []kernelEntry{
		{Version: "6.1.90_1"},
		{Version: "6.6.30_1", Package: "linux6.6"},
		{Version: "6.12.1_1", Running: true, Package: "linux6.12"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries = %+v, want %+v", entries, want)
	}
}

func TestPlutoKernelsStream(t *testing.T) {
	fakeKernelTree(t, "6.1.90_1", "6.6.30_1")
	fakeKernelTools(t, "6.6.30_1")
	out, code := captureStdout(t, func() int { return runPluto([]string{"v2", "kernels"}) })
	if code != 0 {
		t.Fatalf("kernels returned %d: %s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 2 results + done, got %d lines: %s", len(lines), out)
	}
	var first, done struct {
		Signal  string         `json:"signal"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil || first.Signal != "kernels.result" ||
		first.Payload["version"] != "6.1.90_1" || first.Payload["removable"] != true || first.Payload["running"] != false ||
		first.Payload["package"] != "" || first.Payload["leftover"] != true {
		t.Fatalf("bad first record: %s (%v)", lines[0], err)
	}
	if err := json.Unmarshal([]byte(lines[2]), &done); err != nil || done.Signal != "kernels.done" ||
		done.Payload["count"] != float64(2) || done.Payload["running"] != "6.6.30_1" {
		t.Fatalf("bad done record: %s (%v)", lines[2], err)
	}
	if !strings.Contains(lines[1], `"running":true`) || !strings.Contains(lines[1], `"removable":false`) ||
		!strings.Contains(lines[1], `"package":"linux6.6"`) {
		t.Fatalf("running kernel must not be removable: %s", lines[1])
	}
}

func TestPlutoKernelsRejectsArgsAndMissingDirs(t *testing.T) {
	if _, code := captureStdout(t, func() int { return runPluto([]string{"v2", "kernels", "x"}) }); code != 2 {
		t.Fatalf("extra args should exit 2, got %d", code)
	}
	missing := filepath.Join(t.TempDir(), "none")
	oldBoot, oldModules := kernelBootDir, kernelModulesDir
	kernelBootDir, kernelModulesDir = missing, missing
	t.Cleanup(func() { kernelBootDir, kernelModulesDir = oldBoot, oldModules })
	out, code := captureStdout(t, func() int { return runPluto([]string{"v2", "kernels"}) })
	if code != 1 || !strings.Contains(out, "kernel_query_failed") {
		t.Fatalf("unreadable kernel dirs: code=%d out=%s", code, out)
	}
}

func TestRemoveKernelsNativeAndPackage(t *testing.T) {
	root := fakeKernelTree(t, "6.1.90_1", "6.6.30_1", "6.12.1_1")
	tools := fakeKernelTools(t, "6.12.1_1")
	hookLog := filepath.Join(root, "hooks.log")
	fakeExecutable(t, filepath.Join(kernelHooksDir, "post-remove"), "50-log", `echo "$@" >> `+hookLog)
	yesFlag = true
	t.Cleanup(func() { yesFlag = false })

	removeKernels([]string{"6.12.1_1"})
	if _, err := os.Stat(filepath.Join(kernelBootDir, "vmlinuz-6.12.1_1")); err != nil {
		t.Fatal("the running kernel must never be removed")
	}
	removeKernels([]string{"all"})
	if _, err := os.Stat(filepath.Join(kernelBootDir, "vmlinuz-6.1.90_1")); !os.IsNotExist(err) {
		t.Fatal("leftover kernel files should be gone")
	}
	if _, err := os.Stat(filepath.Join(kernelModulesDir, "6.1.90_1")); !os.IsNotExist(err) {
		t.Fatal("leftover modules should be gone")
	}
	if _, err := os.Stat(filepath.Join(kernelBootDir, "vmlinuz-6.6.30_1")); err != nil {
		t.Fatal("'all' must not touch kernels owned by a package")
	}
	if data, _ := os.ReadFile(hookLog); !strings.Contains(string(data), "linux6.1 6.1.90_1") {
		t.Fatalf("post-remove hook not run with package and version: %q", data)
	}
	removeKernels([]string{"6.6.30_1"})
	if data, _ := os.ReadFile(filepath.Join(tools, "xbps-remove.log")); !strings.Contains(string(data), "-y linux6.6") {
		t.Fatalf("package-owned kernel must go through xbps-remove: %q", data)
	}
}
