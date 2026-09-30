package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenableFile(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := openableFile(txt)
	if !ok || got != txt {
		t.Fatalf("path: ok=%v got=%q", ok, got)
	}
	if _, ok := openableFile("discord"); ok {
		t.Fatal("bare program name must stay a query")
	}
	if _, ok := openableFile(filepath.Join(dir, "missing.txt")); ok {
		t.Fatal("missing file")
	}
	if _, ok := openableFile(dir); ok {
		t.Fatal("directory")
	}
}
