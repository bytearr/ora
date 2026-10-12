//go:build !windows

package launch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunHelper(t *testing.T) {
	err := runHelper("sh", "-c", "echo boom >&2; exit 3")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("stderr of a failed helper must be kept: %v", err)
	}
	start := time.Now()
	if err := runHelper("sh", "-c", "sleep 10 & exit 0"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("an app left running by the helper holds its stderr: returned after %s", d)
	}
}

func TestHelperStderrOutlivesHelper(t *testing.T) {
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("no /proc")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	pidFile := filepath.Join(t.TempDir(), "pid")
	start := time.Now()
	spam := "(while :; do echo spam >&2; sleep 0.02; done) & echo $! > " + pidFile + "; exit 0"
	if err := runHelper("sh", "-c", spam); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("runHelper with a writing child returned after %s", d)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if _, err := fmt.Sscan(string(b), &pid); err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)

	fd2 := fmt.Sprintf("/proc/%d/fd/2", pid)
	if link, err := os.Readlink(fd2); err != nil || !strings.HasSuffix(link, "(deleted)") {
		t.Fatalf("child stderr must be an unlinked file: %q %v", link, err)
	}
	size := func() int64 {
		fi, err := os.Stat(fd2)
		if err != nil {
			t.Fatalf("the writer must still run: %v", err)
		}
		return fi.Size()
	}
	s0 := size()
	time.Sleep(300 * time.Millisecond)
	if s1 := size(); s1 <= s0 {
		t.Fatalf("the child must keep writing after runHelper returned: %d -> %d bytes", s0, s1)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Fatalf("temp file left in TMPDIR: %v", left)
	}
}

func TestTerminalArgsFromEnv(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"foot", "kitty", "wezterm", "xfce4-terminal", "myterm"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	for term, want := range map[string]string{
		"foot":                        "foot",
		"kitty":                       "kitty",
		"wezterm":                     "wezterm start --",
		"xfce4-terminal":              "xfce4-terminal -x",
		filepath.Join(dir, "wezterm"): "wezterm start --",
		"myterm":                      "myterm -e",
	} {
		t.Setenv("TERMINAL", term)
		argv, err := terminal()
		if err != nil {
			t.Fatal(err)
		}
		argv[0] = filepath.Base(argv[0])
		if got := strings.Join(argv, " "); got != want {
			t.Errorf("TERMINAL=%s: %q, want %q", term, got, want)
		}
	}
}
