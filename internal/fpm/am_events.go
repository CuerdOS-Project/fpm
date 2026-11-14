package fpm

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const aimgEventPrefix = "FPM_AIMG_EVENT\t"

type aimgEvent struct {
	Protocol        string `json:"protocol"`
	Event           string `json:"event"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	DownloadedBytes int64  `json:"downloaded_bytes,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
}

func aimgEventsEnabled() bool { return os.Getenv("FPM_AIMG_EVENTS") == "1" }

func writeAimgEvent(w io.Writer, event aimgEvent) error {
	event.Protocol = "fpm-aimg-v1"
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s%s\n", aimgEventPrefix, data)
	return err
}

func parseAMFusionProgress(output string) []amFusionPackage {
	packages := parseAMFusionInstalled(output)
	result := make([]amFusionPackage, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, pkg)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func eventNameMatches(name string, requested []string) bool {
	if len(requested) == 0 {
		return true
	}
	nameLower := strings.ToLower(name)
	nameWithoutSuffix := strings.TrimSuffix(nameLower, "-appimage")
	for _, candidate := range requested {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == nameLower || candidate == nameWithoutSuffix || strings.TrimSuffix(candidate, "-appimage") == nameWithoutSuffix {
			return true
		}
	}
	return false
}

func emitAMDownloadEvents(w io.Writer, output string, requested []string, lastSent map[string]int64) {
	for _, pkg := range parseAMFusionProgress(output) {
		key := strings.ToLower(pkg.Name)
		if !eventNameMatches(pkg.Name, requested) {
			continue
		}
		if previous, sent := lastSent[key]; sent && previous == pkg.SizeBytes {
			continue
		}
		if err := writeAimgEvent(w, aimgEvent{Event: "download", Name: pkg.Name, Version: pkg.Version, DownloadedBytes: pkg.SizeBytes}); err != nil {
			return
		}
		lastSent[key] = pkg.SizeBytes
	}
}

func emitAMInstalledEvents(w io.Writer, installed map[string]amFusionPackage, requested []string) {
	for _, pkg := range installed {
		if !eventNameMatches(pkg.Name, requested) {
			continue
		}
		_ = writeAimgEvent(w, aimgEvent{Event: "installed", Name: pkg.Name, Version: pkg.Version, SizeBytes: pkg.SizeBytes})
	}
}

// runAMCommandWithProgress streams the delegated command as usual. Progress
// events are emitted only when explicitly enabled and while an install/update
// child is alive; the external process supervisor and its arguments are untouched.
func runAMCommandWithProgress(args, requested []string) RunResult {
	if !aimgEventsEnabled() || len(args) == 0 || (args[0] != "-ia" && args[0] != "-u") {
		return runAMCommand(args, false)
	}
	result := RunResult{Code: -1}
	backend, err := resolveAMBackend()
	if err != nil {
		result.Err = err
		return result
	}
	cmd := exec.Command(backend, args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=dumb")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		result.Err = err
		return result
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	lastSent := make(map[string]int64)
	for {
		select {
		case err := <-wait:
			result.Err = err
			if cmd.ProcessState != nil {
				result.Code = cmd.ProcessState.ExitCode()
			}
			return result
		case <-ticker.C:
			progress := runAMCommand([]string{"-f", "--byname"}, true)
			if progress.Err == nil {
				emitAMDownloadEvents(os.Stdout, progress.Stdout, requested, lastSent)
			}
		}
	}
}
