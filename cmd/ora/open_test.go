package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ora/internal/index"
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

func TestOpenCommandFlag(t *testing.T) {
	root := newRoot()
	cmd, _, err := root.Find([]string{"open", "-f", "notes.cfg"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "open" {
		t.Fatalf("command %s", cmd.Name())
	}
	f := cmd.Flags().Lookup("file")
	if f == nil || f.Shorthand != "f" {
		t.Fatalf("flag: %+v", f)
	}
}

func TestRevealStoreHasNoPath(t *testing.T) {
	err := (&app{}).reveal(index.Entry{Name: "Calculator", Kind: index.KindStore, Target: "Calc!App", AUMID: "Calc!App"})
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitNoMatch || ee.msg == "" {
		t.Fatalf("%v", err)
	}
}
