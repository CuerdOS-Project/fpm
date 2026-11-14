package fpm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var yelenaUpdateActivityPath = defaultYelenaUpdateActivityPath()

func defaultYelenaUpdateActivityPath() string {
	home, err := os.UserHomeDir()
	// Elevated through sudo/doas/pkexec or Yelena's sudo shim, HOME is /root;
	// the applet watches the invoking user's marker instead.
	if os.Geteuid() == 0 {
		if u, uerr := invokingUser(); uerr == nil && u.HomeDir != "" {
			home, err = u.HomeDir, nil
		}
	}
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "yl-soft", "applet_updating.flag")
}

// yelenaMarkerOwner returns the invoking user's uid/gid when running as root,
// so a marker created for them stays removable and is not owned by root.
func yelenaMarkerOwner() (int, int, bool) {
	if os.Geteuid() != 0 {
		return 0, 0, false
	}
	u, err := invokingUser()
	if err != nil {
		return 0, 0, false
	}
	uid, uerr := strconv.Atoi(u.Uid)
	gid, gerr := strconv.Atoi(u.Gid)
	if uerr != nil || gerr != nil || uid == 0 {
		return 0, 0, false
	}
	return uid, gid, true
}

// beginYelenaUpdateActivity creates the same PID marker consumed by Yelena's
// applet and returns a cleanup function which only removes a marker owned by
// this FPM process. Marker failures never block the package transaction.
func beginYelenaUpdateActivity() func() {
	noop := func() {}
	path := yelenaUpdateActivityPath
	if path == "" {
		return noop
	}
	ownerUID, ownerGID, chownToUser := yelenaMarkerOwner()
	dir := filepath.Dir(path)
	if chownToUser {
		// Running as root inside the user's home: never follow a planted
		// symlink, and only use a directory owned by that user.
		if info, err := os.Lstat(dir); err == nil {
			st, ok := info.Sys().(*syscall.Stat_t)
			if !info.IsDir() || !ok || int(st.Uid) != ownerUID {
				return noop
			}
		} else if os.IsNotExist(err) {
			if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
				return noop
			}
			_ = os.Lchown(dir, ownerUID, ownerGID)
		} else {
			return noop
		}
	} else if err := os.MkdirAll(dir, 0o700); err != nil {
		return noop
	}

	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
		if err == nil {
			if chownToUser {
				_ = file.Chown(ownerUID, ownerGID)
			}
			_, writeErr := fmt.Fprintf(file, "%d", os.Getpid())
			syncErr := file.Sync()
			closeErr := file.Close()
			if writeErr != nil || syncErr != nil || closeErr != nil {
				removeOwnedYelenaMarker(path, os.Getpid())
				return noop
			}
			stopGuard := guardMarkerFromSignals(path)
			return func() {
				stopGuard()
				removeOwnedYelenaMarker(path, os.Getpid())
			}
		}
		if !errors.Is(err, os.ErrExist) {
			return noop
		}

		// An empty or unreadable marker can belong to a process in the middle
		// of writing it; never remove it based on a race-prone guess.
		pid, readErr := readPIDFile(path)
		if readErr != nil {
			return noop
		}
		if markerOwnerAlive(pid) {
			return noop
		}
		_ = os.Remove(path) // stale owner; retry the exclusive creation once
	}
	return noop
}

func processIDAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// markerOwnerAlive reports whether pid still belongs to a running FPM. A bare
// kill(pid, 0) is not enough on a long-lived system: PIDs are recycled, and an
// unrelated process (or an unreaped zombie) would keep a dead marker alive
// forever, leaving the applet stuck on "updating". A live process only counts
// when it is the same program as this one. If /proc cannot be read the check
// stays conservative and assumes the owner is alive.
func markerOwnerAlive(pid int) bool {
	if !processIDAlive(pid) || processIsZombie(pid) {
		return false
	}
	theirs, err1 := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	ours, err2 := os.ReadFile("/proc/self/comm")
	if err1 != nil || err2 != nil {
		return true
	}
	return strings.TrimSpace(string(theirs)) == strings.TrimSpace(string(ours))
}

func processIsZombie(pid int) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	text := string(data)
	// Format: "pid (comm) S ..."; comm may contain spaces or ')', so the
	// state is the field after the last ')'.
	i := strings.LastIndexByte(text, ')')
	if i < 0 || i+2 >= len(text) {
		return false
	}
	return text[i+2] == 'Z' || text[i+2] == 'X'
}

// readPIDFile reads a tiny PID file written by another program. FPM often
// runs as root while reading paths inside a user's home, so this must never
// block on a planted FIFO, follow a planted symlink or read an unbounded file.
func readPIDFile(path string) (int, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("PID file is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, 32))
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, errors.New("invalid PID file contents")
	}
	return pid, nil
}

// guardMarkerFromSignals removes the marker when FPM is ended by SIGINT,
// SIGTERM or SIGHUP. Go's default handling kills the process without running
// deferred cleanup, which would leave the marker behind until the next run.
// After cleaning up, the signal is re-raised with its default disposition so
// the exit status seen by the caller is unchanged. The returned function
// stops the guard and is safe to call more than once.
func guardMarkerFromSignals(path string) func() {
	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		select {
		case sig := <-ch:
			removeOwnedYelenaMarker(path, os.Getpid())
			signal.Reset(sig)
			if number, ok := sig.(syscall.Signal); ok {
				_ = syscall.Kill(os.Getpid(), number)
				time.Sleep(time.Second)
				os.Exit(128 + int(number))
			}
			os.Exit(1)
		case <-done:
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			signal.Stop(ch)
			close(done)
		})
	}
}

func removeOwnedYelenaMarker(path string, pid int) {
	owner, err := readPIDFile(path)
	if err != nil || owner != pid {
		return
	}
	_ = os.Remove(path)
}
