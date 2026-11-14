package fpm

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

// resolveAMBackend honors the frontend selected when am-FUSION was installed.
// Without a saved preference it uses AppMan first, keeping GUI package actions
// in the user account instead of unexpectedly selecting system-wide AM/sudo.
func resolveAMBackend() (string, error) {
	preferred := configuredAMBackend()
	if preferred != "" {
		if path, ok := findAMBackend(preferred); ok {
			return path, nil
		}
		return "", fmt.Errorf("el backend AM seleccionado (%s) ya no está disponible", preferred)
	}
	for _, name := range []string{"appman", "am"} {
		if path, ok := findAMBackend(name); ok {
			return path, nil
		}
	}
	return "", fmt.Errorf("no se encontró AM ni AppMan en PATH ni en la ruta local")
}

func findAMBackend(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	if name == "appman" {
		if path := appManInstallPath(); path != "" {
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
				return path, true
			}
		}
	}
	return "", false
}

// runAMCommand executes the real AM/AppMan backend without a shell. Captured
// calls are used only for FPM's synchronized catalogue and inventory cache.
func runAMCommand(args []string, capture bool) RunResult {
	result := RunResult{Code: -1}
	backend, err := resolveAMBackend()
	if err != nil {
		result.Err = err
		return result
	}
	cmd := exec.Command(backend, args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=dumb")
	cmd.Stdin = os.Stdin
	if capture {
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = os.Stderr
		result.Err = cmd.Run()
		result.Stdout = stdout.String()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		result.Err = cmd.Run()
	}
	if cmd.ProcessState != nil {
		result.Code = cmd.ProcessState.ExitCode()
	}
	return result
}

// runAMBackend retains the old `fpm am ...` spelling as a compatibility alias.
// New AppImage operations use the unified `fpm aimg` command group.
func runAMBackend(args []string) {
	result := runAMCommand(args, false)
	if result.Err != nil {
		if result.Code >= 0 {
			os.Exit(result.Code)
		}
		fmt.Fprintf(os.Stderr, "FPM: no se pudo ejecutar AM/AppMan: %v\n", result.Err)
		os.Exit(1)
	}
	if amArgumentsModifyApps(args) {
		if err := refreshAMFusionInstalledSnapshot(); err != nil {
			fmt.Fprintf(os.Stderr, "FPM: aviso: no se pudo sincronizar am-FUSION: %v\n", err)
		}
	}
}

func amArgumentsModifyApps(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-i", "-ia", "-ias", "install", "install-appimage", "-r", "-R", "remove", "-u", "-U", "update", "upgrade":
			return true
		}
	}
	return false
}
