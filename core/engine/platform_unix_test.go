//go:build !windows

package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileIndexRetriesAfterFailure(t *testing.T) {
	e := testEngine(t)
	root := filepath.Join(t.TempDir(), "later")
	e.Cfg.Files.Roots = []string{root}
	e.Cfg.Files.Rescan = time.Hour
	if _, err := e.fileIndex(); err == nil {
		t.Fatal("a missing root must fail")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "found.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ix, err := e.fileIndex()
	if err != nil || ix == nil || ix.Len() != 1 {
		t.Fatalf("second try: %v", err)
	}
	if again, _ := e.fileIndex(); again != ix {
		t.Fatal("a working index is opened once")
	}
}
