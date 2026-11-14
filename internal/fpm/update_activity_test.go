package fpm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func withYelenaMarkerPath(t *testing.T) string {
	t.Helper()
	old := yelenaUpdateActivityPath
	path := filepath.Join(t.TempDir(), "config", "yl-soft", "applet_updating.flag")
	yelenaUpdateActivityPath = path
	t.Cleanup(func() { yelenaUpdateActivityPath = old })
	return path
}

func TestYelenaUpdateActivityMarkerLifecycle(t *testing.T) {
	path := withYelenaMarkerPath(t)
	cleanup := beginYelenaUpdateActivity()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("marker was not created: %v", err)
	}
	if got := strings.TrimSpace(string(contents)); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("marker PID = %q, want %d", got, os.Getpid())
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("marker still exists after owner cleanup, stat err=%v", err)
	}
}

func TestYelenaUpdateMarkerIsExclusiveAndOwnerScoped(t *testing.T) {
	path := withYelenaMarkerPath(t)
	cleanupOwner := beginYelenaUpdateActivity()
	cleanupOther := beginYelenaUpdateActivity()
	cleanupOther()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("second attempt removed the active marker: %v", err)
	}
	if got := strings.TrimSpace(string(contents)); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("active marker changed unexpectedly: %q", got)
	}
	cleanupOwner()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owner cleanup did not remove marker, stat err=%v", err)
	}
}

func TestYelenaUpdateMarkerReplacesStalePID(t *testing.T) {
	path := withYelenaMarkerPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("2147483647"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanup := beginYelenaUpdateActivity()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("stale marker was not replaced: %v", err)
	}
	if got := strings.TrimSpace(string(contents)); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("marker PID = %q, want %d", got, os.Getpid())
	}
	cleanup()
}

func TestYelenaUpdateMarkerReplacesRecycledPID(t *testing.T) {
	ours, err1 := os.ReadFile("/proc/self/comm")
	theirs, err2 := os.ReadFile("/proc/1/comm")
	if err1 != nil || err2 != nil || strings.TrimSpace(string(ours)) == strings.TrimSpace(string(theirs)) {
		t.Skip("cannot tell PID 1 apart from the test binary")
	}
	path := withYelenaMarkerPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// PID 1 is alive but is not FPM: a recycled PID must not pin the marker.
	if err := os.WriteFile(path, []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanup := beginYelenaUpdateActivity()
	defer cleanup()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("marker disappeared: %v", err)
	}
	if got := strings.TrimSpace(string(contents)); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("recycled-PID marker was not replaced: %q", got)
	}
}

func TestMarkerOwnerAliveRecognisesSelfAndDeadPIDs(t *testing.T) {
	if !markerOwnerAlive(os.Getpid()) {
		t.Fatal("current process must count as a live owner")
	}
	if markerOwnerAlive(2147483647) {
		t.Fatal("non-existent PID must not count as a live owner")
	}
}

func TestReadPIDFileRejectsUnsafeFiles(t *testing.T) {
	dir := t.TempDir()

	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := readPIDFile(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a FIFO was accepted as a PID file")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readPIDFile blocked on a FIFO")
	}

	target := filepath.Join(dir, "real")
	if err := os.WriteFile(target, []byte("123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readPIDFile(link); err == nil {
		t.Fatal("a symlink was followed")
	}
	if pid, err := readPIDFile(target); err != nil || pid != 123 {
		t.Fatalf("readPIDFile(real) = %d, %v; want 123, nil", pid, err)
	}
}

func TestReadPIDFileRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"text":     "abc",
		"zero":     "0",
		"negative": "-5",
		"empty":    "",
		"huge":     strings.Repeat("9", 4096),
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if pid, err := readPIDFile(path); err == nil {
			t.Errorf("%s: accepted garbage PID %d", name, pid)
		}
	}
}

func TestGuardMarkerFromSignalsStopIsIdempotent(t *testing.T) {
	stop := guardMarkerFromSignals(filepath.Join(t.TempDir(), "marker"))
	stop()
	stop()
}

func TestSelfExecutableIsUsable(t *testing.T) {
	exe := selfExecutable()
	if exe == "" || strings.HasSuffix(exe, " (deleted)") {
		t.Fatalf("selfExecutable() = %q", exe)
	}
}

// The child half of TestMarkerRemovedWhenTerminatedBySignal: create the marker
// and wait to be killed.
func TestMarkerRemovedWhenTerminatedBySignal(t *testing.T) {
	if os.Getenv("FPM_TEST_MARKER_CHILD") == "1" {
		yelenaUpdateActivityPath = os.Getenv("FPM_TEST_MARKER_PATH")
		beginYelenaUpdateActivity()
		time.Sleep(time.Minute)
		return
	}
	path := withYelenaMarkerPath(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestMarkerRemovedWhenTerminatedBySignal$")
	cmd.Env = append(os.Environ(), "FPM_TEST_MARKER_CHILD=1", "FPM_TEST_MARKER_PATH="+path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	want := strconv.Itoa(cmd.Process.Pid)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if pid, err := readPIDFile(path); err == nil && strconv.Itoa(pid) == want {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatal("child never created its marker")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("child exit = %v, want termination by signal", err)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
		t.Fatalf("child status = %v, want killed by SIGTERM", exitErr)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("marker survived SIGTERM, stat err=%v", statErr)
	}
}
