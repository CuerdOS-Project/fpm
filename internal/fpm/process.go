package fpm

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

type RunResult struct {
	Code   int
	Stdout string
	Stderr string
	Err    error
}

var (
	verbose bool
	yesFlag bool
)

// runCmd ejecuta directamente el binario, sin shell ni interpolación. Esto
// conserva el comportamiento de execvp() y evita que los argumentos del
// usuario sean interpretados como sintaxis de shell.
func runCmd(args []string, capture bool) RunResult {
	result := RunResult{Code: -1}
	if len(args) == 0 {
		result.Err = errors.New("comando vacío")
		return result
	}
	if verbose && !capture {
		dInfo("%s", tr("info_operation_running"))
	}

	cmd := exec.Command(args[0], args[1:]...)
	if capture {
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = os.Stderr
		// Run synchronously without a timeout: package managers may take a long
		// time, and FPM must keep waiting instead of terminating the backend.
		err := cmd.Run()
		result.Stdout = stdout.String()
		result.Err = err
	} else {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// Stream output and wait synchronously, regardless of how long the
		// backend needs. Do not add a duration-based timeout here.
		result.Err = cmd.Run()
	}
	if cmd.ProcessState != nil {
		result.Code = cmd.ProcessState.ExitCode()
	}
	return result
}

func cmdExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

var rootCommands = map[string]bool{
	"install": true, "remove": true, "superremove": true,
	"update": true, "upgrade": true, "repair": true,
	"clean": true, "check": true, "hold": true,
	"unhold": true, "autoremove": true, "db": true,
}

// yelenaSudoShimDirPrefix is the session directory prefix of the `sudo` shim
// that Yelena Software puts first in PATH (backend/priv_broker.py). The shim
// forwards the command to a single pkexec-authorised broker, so FPM must call
// `sudo` (not `doas`) whenever that shim is the sudo being resolved.
const yelenaSudoShimDirPrefix = "yelena-aimg-"

func isYelenaSudoShim(path string) bool {
	return filepath.Base(path) == "sudo" &&
		strings.HasPrefix(filepath.Base(filepath.Dir(path)), yelenaSudoShimDirPrefix)
}

// chooseElevator picks the privilege tool: FPM_ELEVATOR=sudo|doas wins, then
// the Yelena sudo shim, then doas, then sudo.
func chooseElevator() string {
	switch forced := strings.TrimSpace(os.Getenv("FPM_ELEVATOR")); forced {
	case "sudo", "doas":
		if cmdExists(forced) {
			return forced
		}
	}
	if path, err := exec.LookPath("sudo"); err == nil && isYelenaSudoShim(path) {
		return "sudo"
	}
	if cmdExists("doas") {
		return "doas"
	}
	if cmdExists("sudo") {
		return "sudo"
	}
	return ""
}

// elevateIfNeeded reemplaza al proceso actual con sudo/doas, evitando
// construir una línea de comandos para un shell.
func elevateIfNeeded(command string) {
	if os.Geteuid() == 0 || !rootCommands[command] {
		return
	}
	elevator := chooseElevator()
	if elevator == "" {
		dErr("%s", tr("err_no_privilege_tool"))
		os.Exit(1)
	}
	exe := selfExecutable()
	dInfo(tr("info_elevating"), command, elevator)
	elevatorPath, err := exec.LookPath(elevator)
	if err != nil {
		dErr("%s: %v", tr("err_elevation_failed"), err)
		os.Exit(1)
	}
	args := append([]string{elevator, exe}, os.Args[1:]...)
	if err := syscall.Exec(elevatorPath, args, os.Environ()); err != nil {
		dErr("%s: %v", tr("err_elevation_failed"), err)
		os.Exit(1)
	}
}

// selfExecutable returns an absolute path to this binary for re-executing it
// under sudo/doas. os.Executable can fail or carry a " (deleted)" suffix when
// the package manager replaced the binary while it was running; a bare
// os.Args[0] would then be resolved against the elevator's own PATH.
func selfExecutable() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		return strings.TrimSuffix(exe, " (deleted)")
	}
	if found, err := exec.LookPath(os.Args[0]); err == nil {
		if abs, absErr := filepath.Abs(found); absErr == nil {
			return abs
		}
		return found
	}
	return os.Args[0]
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func joinArgs(items []string) string {
	return strings.Join(items, " ")
}
