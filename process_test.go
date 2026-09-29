package main

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunCmdWaitsForLongRunningBackendAndHidesCommand(t *testing.T) {
	oldVerbose, oldStdout := verbose, os.Stdout
	defer func() {
		verbose = oldVerbose
		os.Stdout = oldStdout
	}()
	verbose = true

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer

	started := time.Now()
	result := runCmd([]string{"sh", "-c", "sleep 0.15; printf backend-finished"}, false)
	elapsed := time.Since(started)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != 0 {
		t.Fatalf("backend returned code %d: %v", result.Code, result.Err)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("runCmd returned before the backend finished: elapsed %s", elapsed)
	}
	text := string(output)
	if !strings.Contains(text, "backend-finished") {
		t.Fatalf("backend output was not streamed: %q", text)
	}
	if strings.Contains(text, "sh -c") || strings.Contains(text, "sleep 0.15") {
		t.Fatalf("raw backend command was printed: %q", text)
	}
}
