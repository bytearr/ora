package files

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestSaveGeneration(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "files.idx")
	ix := New(nil, nil)
	ix.mu.Lock()
	ix.add("/old.txt", false)
	ix.mu.Unlock()

	writeFile := ix.writeFile
	ix.writeFile = func(path string, data []byte) error {
		ix.mu.Lock()
		ix.add("/new.txt", false)
		ix.mu.Unlock()
		return writeFile(path, data)
	}
	if err := ix.Save(cache); err != nil {
		t.Fatal(err)
	}
	if !ix.isDirty() {
		t.Fatal("concurrent change was marked clean")
	}

	ix.writeFile = writeFile
	if err := ix.Save(cache); err != nil {
		t.Fatal(err)
	}
	if ix.isDirty() {
		t.Fatal("uninterrupted save left index dirty")
	}
	back := New(nil, nil)
	if ok, err := back.Load(cache); !ok || err != nil {
		t.Fatalf("load: %v %v", ok, err)
	}
	if hits, _ := back.Search("new", false, false, 0); len(hits) != 1 {
		t.Fatal("second save did not persist concurrent change")
	}

	ix.mu.Lock()
	ix.add("/failed.txt", false)
	ix.mu.Unlock()
	ix.writeFile = func(string, []byte) error {
		return errors.New("write failed")
	}
	if err := ix.Save(cache); err == nil {
		t.Fatal("save succeeded")
	}
	if !ix.isDirty() {
		t.Fatal("failed save marked index clean")
	}
}
