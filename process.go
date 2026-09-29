package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
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

// elevateIfNeeded reemplaza al proceso actual con sudo/doas, evitando
// construir una línea de comandos para un shell.
func elevateIfNeeded(command string) {
	if os.Geteuid() == 0 || !rootCommands[command] {
		return
	}
	elevator := ""
	if cmdExists("doas") {
		elevator = "doas"
	} else if cmdExists("sudo") {
		elevator = "sudo"
	}
	if elevator == "" {
		dErr("%s", tr("err_no_privilege_tool"))
		os.Exit(1)
	}
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
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
