package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLIEntrypointVersionAndHelp(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate integration test source")
	}
	root := filepath.Dir(filepath.Dir(sourceFile))
	binary := filepath.Join(t.TempDir(), "fpm")

	build := exec.Command("go", "build", "-o", binary, "./cmd/fpm")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build FPM CLI: %v\n%s", err, output)
	}

	for _, flag := range []string{"--version", "-V"} {
		version := exec.Command(binary, flag)
		version.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")
		if output, err := version.CombinedOutput(); err != nil {
			t.Fatalf("run %s: %v\n%s", flag, err, output)
		} else if strings.TrimSpace(string(output)) != "FPM 2.2" {
			t.Fatalf("unexpected version output for %s: %s", flag, output)
		}
	}

	help := exec.Command(binary, "--help")
	help.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")
	if output, err := help.CombinedOutput(); err != nil {
		t.Fatalf("run --help: %v\n%s", err, output)
	} else {
		for _, command := range []string{"update", "upgrade", "repos", "aimg repo", "flat install"} {
			if !strings.Contains(string(output), command) {
				t.Errorf("help output does not include %q:\n%s", command, output)
			}
		}
	}

	badAppImage := exec.Command(binary, "aimg", "install")
	badAppImage.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")
	if output, err := badAppImage.CombinedOutput(); err == nil {
		t.Fatalf("invalid AppImage operation unexpectedly succeeded:\n%s", output)
	} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("invalid AppImage operation should exit 1, got %v:\n%s", err, output)
	}
}
