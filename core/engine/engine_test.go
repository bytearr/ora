package engine

import (
	"errors"
	"testing"
	"time"

	"ora/core/config"
	"ora/core/index"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("LOCALAPPDATA", dir)
	return &Engine{Cfg: &config.Config{Aliases: map[string]string{}, IndexTTL: 24 * time.Hour}}
}

func memIndex() *index.Index {
	return &index.Index{Entries: []index.Entry{
		{Name: "Visual Studio Code", Kind: index.KindShortcut, Source: index.SourceStartMenu, Target: `C:\lnk\code.lnk`},
		{Name: "Discord", Kind: index.KindShortcut, Source: index.SourceStartMenu, Target: `C:\lnk\discord.lnk`},
		{Name: "Disk Cleanup", Kind: index.KindShortcut, Source: index.SourceStartMenu, Target: `C:\lnk\cleanmgr.lnk`},
		{Name: "Calculator", Kind: index.KindStore, Source: index.SourceAppsFolder, Target: "Calc!App", AUMID: "Calc!App"},
	}}
}

func TestSearchFullList(t *testing.T) {
	e := testEngine(t)
	ix := memIndex()
	rs := e.Search(ix, "discord")
	if len(rs) != len(ix.Entries) {
		t.Fatalf("want every entry ranked, got %d", len(rs))
	}
	if got := ix.Entries[rs[0].Index].Name; got != "Discord" {
		t.Fatalf("winner %q", got)
	}
}

func TestSearchAlias(t *testing.T) {
	e := testEngine(t)
	e.Cfg.Aliases["vsc"] = "Visual Studio Code"
	ix := memIndex()
	rs := e.Search(ix, "vsc")
	if got := ix.Entries[rs[0].Index].Name; got != "Visual Studio Code" {
		t.Fatalf("alias winner %q", got)
	}
}

func TestSearchRecentBonus(t *testing.T) {
	e := testEngine(t)
	ix := memIndex()
	before := e.Search(ix, "dis")
	if before[0].Score != before[1].Score {
		t.Fatalf("setup: prefix tie expected, got %v", before[:2])
	}
	cleanup := ix.Entries[2]
	e.Remember(cleanup)
	ids, err := e.RecentIDs()
	if err != nil || len(ids) != 1 || ids[0] != cleanup.ID() {
		t.Fatalf("recent %v %v", ids, err)
	}
	after := e.Search(ix, "dis")
	if got := ix.Entries[after[0].Index].Name; got != "Disk Cleanup" {
		t.Fatalf("recent entry must win the tie, got %q", got)
	}
}

func TestRememberFrontAndID(t *testing.T) {
	e := testEngine(t)
	f := index.Entry{Name: "notes.txt", Kind: index.KindFile, Target: `E:\Docs\Notes.TXT`}
	s := index.Entry{Name: "Calculator", Kind: index.KindStore, AUMID: "Calc!App"}
	e.Remember(f)
	e.Remember(s)
	e.Remember(f)
	ids, _ := e.RecentIDs()
	want := []string{`path:e:\docs\notes.txt`, "aumid:calc!app"}
	if len(ids) != 2 || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("got %v", ids)
	}
}

func TestRevealStoreHasNoPath(t *testing.T) {
	e := testEngine(t)
	err := e.Reveal(index.Entry{Name: "Calculator", Kind: index.KindStore, Target: "Calc!App", AUMID: "Calc!App"})
	if !errors.Is(err, ErrNoPath) {
		t.Fatalf("%v", err)
	}
}
