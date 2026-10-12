//go:build !windows

// The exclude patterns use "/" separators; on Windows ora uses Everything instead.

package files

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func tree(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if p[len(p)-1] == '/' {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func paths(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Path
	}
	return out
}

func TestScanSearchExclude(t *testing.T) {
	root := tree(t, "Docs/Project Notes.txt", "Docs/b-notes.md", "work/notes/", "node_modules/x/notes.js", "Apps/Tool.AppImage")
	ix := New([]string{root}, []*regexp.Regexp{regexp.MustCompile(`(?i)/node_modules/`)})
	if err := ix.Scan(); err != nil {
		t.Fatal(err)
	}
	hits, total := ix.Search("NOTES", false, false, 0)
	want := []string{filepath.Join(root, "Docs/b-notes.md"), filepath.Join(root, "Docs/Project Notes.txt")}
	if total != 2 || len(hits) != 2 || hits[0].Path != want[0] || hits[1].Path != want[1] {
		t.Fatalf("files: %v (%d)", paths(hits), total)
	}
	hits, total = ix.Search("notes", true, false, 1)
	if total != 3 || len(hits) != 1 {
		t.Fatalf("folders and cap: %v (%d)", paths(hits), total)
	}
	if hits, _ := ix.Search("docs project", false, false, 0); len(hits) != 0 {
		t.Fatalf("names only: %v", paths(hits))
	}
	if hits, _ := ix.Search(`docs "project"`, false, true, 0); len(hits) != 1 {
		t.Fatalf("path match: %v", paths(hits))
	}
	if got := ix.WithExt([]string{".appimage"}); len(got) != 1 || filepath.Base(got[0]) != "Tool.AppImage" {
		t.Fatalf("ext: %v", got)
	}
}

func TestSaveLoadKey(t *testing.T) {
	root := tree(t, "a/b.txt", "c.txt")
	cache := filepath.Join(t.TempDir(), "files.idx")
	ix := New([]string{root}, nil)
	if err := ix.Scan(); err != nil {
		t.Fatal(err)
	}
	if err := ix.Save(cache); err != nil {
		t.Fatal(err)
	}
	back := New([]string{root}, nil)
	if ok, err := back.Load(cache); !ok || err != nil || back.Len() != ix.Len() || back.Stale(time.Hour) {
		t.Fatalf("load: %v %v %d/%d", ok, err, back.Len(), ix.Len())
	}
	other := New([]string{root}, []*regexp.Regexp{regexp.MustCompile("x")})
	if ok, _ := other.Load(cache); ok {
		t.Fatal("cache with other excludes must be ignored")
	}
}

func TestRemoveTree(t *testing.T) {
	ix := New(nil, nil)
	for _, p := range []string{"/r/a", "/r/a/x", "/r/a/y/z", "/r/ab", "/r/b"} {
		ix.add(p, p == "/r/a" || p == "/r/a/y")
	}
	ix.remove("/r/a")
	if got := ix.Len(); got != 2 {
		t.Fatalf("left %d", got)
	}
	if _, ok := ix.pos["/r/ab"]; !ok {
		t.Fatal("sibling with same prefix removed")
	}
	for p, i := range ix.pos {
		if ix.entries[i].path != p {
			t.Fatalf("pos out of sync for %s", p)
		}
	}
}

func TestWatch(t *testing.T) {
	root := tree(t, "old.txt", "skip/")
	cache := filepath.Join(t.TempDir(), "files.idx")
	ix := New([]string{root}, []*regexp.Regexp{regexp.MustCompile(`/skip/`)})
	if err := ix.Scan(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ix.Watch(ctx, cache, 0, t.Logf)
		close(done)
	}()
	time.Sleep(200 * time.Millisecond) // watches are added

	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "new/deep"), 0o755))
	must(os.WriteFile(filepath.Join(root, "new/deep/fresh.txt"), nil, 0o644))
	must(os.WriteFile(filepath.Join(root, "skip/hidden.txt"), nil, 0o644))
	must(os.Rename(filepath.Join(root, "old.txt"), filepath.Join(root, "renamed.txt")))

	wait := func(q string, n int) {
		t.Helper()
		for i := 0; i < 50; i++ {
			if hits, _ := ix.Search(q, false, false, 0); len(hits) == n {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		hits, _ := ix.Search(q, false, false, 0)
		t.Fatalf("%q: %v, want %d", q, paths(hits), n)
	}
	wait("fresh", 1)
	wait("renamed", 1)
	wait("old", 0)
	wait("hidden", 0)

	must(os.RemoveAll(filepath.Join(root, "new")))
	wait("fresh", 0)

	cancel()
	<-done
	back := New([]string{root}, ix.exclude)
	if ok, _ := back.Load(cache); !ok {
		t.Fatal("watch must save on exit")
	}
	if hits, _ := back.Search("renamed", false, false, 0); len(hits) != 1 {
		t.Fatal("saved index lacks the change")
	}
}
