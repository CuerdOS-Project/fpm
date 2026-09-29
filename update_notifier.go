package main

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// notifyYelenaUpdateManager asks the current user's Yelena applet to refresh
// its update cache after an FPM transaction. The PID file is only a hint:
// before signalling, verify both the target UID and the process command line.
func notifyYelenaUpdateManager() {
	u, err := invokingUser()
	if err != nil || u.HomeDir == "" {
		return
	}
	pidPath := filepath.Join(u.HomeDir, ".config", "yl-soft", "applet.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 {
		return
	}
	procPath := filepath.Join("/proc", strconv.Itoa(pid))
	info, err := os.Stat(procPath)
	if err != nil {
		return
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil || int(stat.Uid) != uid {
		return
	}
	command, err := os.ReadFile(filepath.Join(procPath, "cmdline"))
	if err != nil || !isYelenaAppletCommand(string(command)) {
		return
	}
	process, err := os.FindProcess(pid)
	if err == nil {
		_ = process.Signal(syscall.SIGUSR2)
	}
}

func invokingUser() (*user.User, error) {
	for _, name := range []string{"SUDO_USER", "DOAS_USER"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" && value != "root" {
			if u, err := user.Lookup(value); err == nil {
				return u, nil
			}
		}
	}
	for _, name := range []string{"SUDO_UID", "PKEXEC_UID"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" && value != "0" {
			if u, err := user.LookupId(value); err == nil {
				return u, nil
			}
		}
	}
	if os.Geteuid() != 0 {
		return user.Current()
	}
	return nil, os.ErrNotExist
}

func isYelenaAppletCommand(commandLine string) bool {
	for _, arg := range strings.Split(commandLine, "\x00") {
		if strings.HasSuffix(arg, "/applet.py") || arg == "applet.py" || arg == "yl-soft-applet" {
			return true
		}
	}
	return false
}
