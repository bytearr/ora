package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	utf16enc "unicode/utf16"

	"ora/core/index"
	"ora/core/match"
)

func TestWordStart(t *testing.T) {
	cases := []struct {
		text  string
		caret int
		want  int
	}{
		{"", 0, 0},
		{"discord", 7, 0},
		{"open discord", 12, 5},
		{"open discord  ", 14, 5},
		{"open discord", 4, 0},
		{`C:\Users\foo`, 12, 9},
		{`C:\Users\`, 9, 8},
		{`C:\Users`, 8, 3},
		{"a.txt", 5, 2},
		{"   ", 3, 0},
	}
	for _, c := range cases {
		u := utf16enc.Encode([]rune(c.text))
		if got := wordStart(u, c.caret); got != c.want {
			t.Errorf("wordStart(%q, %d) = %d, want %d", c.text, c.caret, got, c.want)
		}
	}
}

func progs(n int) []Row {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{Entry: index.Entry{Name: fmt.Sprintf("app%02d", i), Kind: index.KindShortcut, Target: fmt.Sprintf(`C:\lnk\app%02d.lnk`, i)}}
	}
	return rows
}

func withRows(rows []Row) *Model {
	m := &Model{}
	m.SetQuery("ab", false)
	m.Apply(Result{Gen: m.Gen, Rows: rows})
	return m
}

func TestQueryMode(t *testing.T) {
	cases := map[string]Mode{"": ModeRecent, "  ": ModeRecent, "a": ModeNone, " ä ": ModeNone, "ab": ModeSearch, "äö": ModeSearch}
	for q, want := range cases {
		if got := QueryMode(q); got != want {
			t.Errorf("%q: %v want %v", q, got, want)
		}
	}
}

func TestStaleGenerationDropped(t *testing.T) {
	m := &Model{}
	m.SetQuery("di", false)
	old := m.Gen
	m.SetQuery("dis", false)
	if m.Apply(Result{Gen: old, Rows: progs(3)}) {
		t.Fatal("old generation applied")
	}
	if len(m.Rows) != 0 {
		t.Fatal("rows changed by stale result")
	}
	if !m.Apply(Result{Gen: m.Gen, Rows: progs(2)}) || len(m.Rows) != 2 {
		t.Fatal("current generation not applied")
	}
}

func TestKeepRowsUntilResult(t *testing.T) {
	m := withRows(progs(8))
	m.Move(2)
	m.SetQuery("abc", true)
	if !m.Stale || len(m.Rows) != 8 || m.Sel != 0 {
		t.Fatalf("stale %v rows %d sel %d", m.Stale, len(m.Rows), m.Sel)
	}
	if a := m.Activate(false); a.Kind != ActNone {
		t.Fatalf("enter on stale rows must wait: %+v", a)
	}
	if a := m.TakePending(); a.Kind != ActNone {
		t.Fatal("pending fired before the result")
	}
	fresh := progs(3)
	fresh[0].Entry.Name = "fresh"
	m.Apply(Result{Gen: m.Gen, Rows: fresh})
	if a := m.TakePending(); a.Kind != ActLaunch || a.Entry.Name != "fresh" {
		t.Fatalf("pending enter on the fresh result: %+v", a)
	}
	if a := m.TakePending(); a.Kind != ActNone {
		t.Fatal("pending fired twice")
	}
}

func TestPendingRevealAndReset(t *testing.T) {
	m := withRows(progs(3))
	m.SetQuery("abc", true)
	m.Activate(true)
	m.Apply(Result{Gen: m.Gen, Rows: progs(2)})
	if a := m.TakePending(); a.Kind != ActReveal {
		t.Fatalf("shift+enter must stay a reveal: %+v", a)
	}

	m.SetQuery("abcd", true)
	m.Activate(false)
	m.SetQuery("abcde", true) // typing after Enter drops it
	m.Apply(Result{Gen: m.Gen, Rows: progs(2)})
	if a := m.TakePending(); a.Kind != ActNone {
		t.Fatalf("enter survived a new keystroke: %+v", a)
	}

	m.SetQuery("", false)
	if m.Stale || len(m.Rows) != 0 {
		t.Fatal("without keep the list must be empty")
	}
}

func TestClickOnStaleRow(t *testing.T) {
	m := withRows(progs(3))
	m.SetQuery("abc", true)
	if a := m.Click(1, false); a.Kind != ActLaunch || a.Entry.Name != "app01" {
		t.Fatalf("click starts the row on screen: %+v", a)
	}
}

func TestDropMissing(t *testing.T) {
	rows := []Row{
		{Entry: index.Entry{Name: "gone", Kind: index.KindPortable, Target: `C:\gone.exe`}},
		{Entry: index.Entry{Name: "here", Kind: index.KindPortable, Target: `C:\here.exe`}},
		{Entry: index.Entry{Name: "lnk", Kind: index.KindShortcut, Target: `C:\gone.lnk`}},
	}
	stat := func(p string) (os.FileInfo, error) {
		if p == `C:\here.exe` {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	got := DropMissing(rows, stat)
	if len(got) != 2 || got[0].Entry.Name != "here" || got[1].Entry.Name != "lnk" {
		t.Fatalf("%+v", got)
	}
	if len(rows) != 3 || rows[0].Entry.Name != "gone" {
		t.Fatal("input changed")
	}
}

func TestCollapsedShowMore(t *testing.T) {
	m := withRows(progs(8))
	items := m.Items()
	if len(items) != visibleRows+1 || !items[visibleRows].More {
		t.Fatalf("want 5 hits plus the button, got %+v", items)
	}
	m.Move(10)
	if m.Sel != visibleRows {
		t.Fatalf("down from hit 5 must land on the button, sel %d", m.Sel)
	}
	if a := m.Activate(false); a.Kind != ActExpand {
		t.Fatalf("enter on Show more: %+v", a)
	}
	if !m.Expanded || len(m.Items()) != 8 {
		t.Fatalf("expanded list %d", len(m.Items()))
	}
	for _, it := range m.Items() {
		if it.More {
			t.Fatal("button must be gone")
		}
	}
	if a := m.Activate(false); a.Kind != ActLaunch || a.Entry.Name != "app05" {
		t.Fatalf("after expand the selection is on hit 6: %+v", a)
	}
}

func TestNoButtonForFiveOrLess(t *testing.T) {
	m := withRows(progs(5))
	if items := m.Items(); len(items) != 5 || items[4].More {
		t.Fatalf("%+v", items)
	}
	m.Move(10)
	if m.Sel != 4 {
		t.Fatalf("sel %d", m.Sel)
	}
}

func TestExpandedScrollAndFollow(t *testing.T) {
	m := withRows(progs(30))
	m.Move(visibleRows)
	m.Activate(false)
	first, vis := m.Visible()
	if first != 0 || len(vis) != expandedRows {
		t.Fatalf("first %d len %d", first, len(vis))
	}
	m.Move(20)
	if m.Sel != 25 || m.Top != 25-expandedRows+1 {
		t.Fatalf("sel %d top %d", m.Sel, m.Top)
	}
	m.Move(-100)
	if m.Sel != 0 || m.Top != 0 {
		t.Fatalf("sel %d top %d", m.Sel, m.Top)
	}
	m.Scroll(100)
	if m.Top != 30-expandedRows || m.Sel != 0 {
		t.Fatalf("scroll top %d sel %d", m.Top, m.Sel)
	}
	m.Scroll(-3)
	if m.Top != 30-expandedRows-3 {
		t.Fatalf("top %d", m.Top)
	}
	m.SetQuery("abc", false)
	m.Apply(Result{Gen: m.Gen, Rows: progs(30)})
	if m.Expanded || m.Sel != 0 || m.Top != 0 || len(m.Items()) != visibleRows+1 {
		t.Fatal("new query must collapse and reset the selection")
	}
}

func TestScrollCollapsedDoesNothing(t *testing.T) {
	m := withRows(progs(30))
	m.Scroll(3)
	if m.Top != 0 {
		t.Fatal("collapsed list scrolled")
	}
}

func TestClick(t *testing.T) {
	m := withRows(progs(20))
	if a := m.Click(1, false); a.Kind != ActLaunch || a.Entry.Name != "app01" || m.Sel != 1 {
		t.Fatalf("click row 2: %+v", a)
	}
	if a := m.Click(visibleRows, false); a.Kind != ActExpand {
		t.Fatalf("click Show more: %+v", a)
	}
	m.Scroll(2)
	if a := m.Click(0, false); a.Entry.Name != "app02" {
		t.Fatalf("click after scroll: %+v", a)
	}
	if a := m.Click(99, false); a.Kind != ActNone {
		t.Fatal("click below the list")
	}
}

func TestEmptyListActivate(t *testing.T) {
	m := &Model{}
	m.SetQuery("x", false)
	if a := m.Activate(false); a.Kind != ActNone {
		t.Fatal(a)
	}
	m.Move(1)
}

func programIndex(names ...string) *index.Index {
	ix := &index.Index{}
	for _, n := range names {
		ix.Entries = append(ix.Entries, index.Entry{Name: n, Kind: index.KindShortcut, Target: `C:\lnk\` + n + `.lnk`})
	}
	return ix
}

func TestProgramRows(t *testing.T) {
	ix := programIndex("Discord", "Discord", "Disk Cleanup", "Paint")
	rs := []match.Result{{Index: 0, Score: 1}, {Index: 1, Score: 1}, {Index: 2, Score: 0.6}, {Index: 3, Score: 0.2}}
	rows := ProgramRows(ix, rs)
	if len(rows) != 3 || rows[0].Blocked || rows[1].Blocked {
		t.Fatalf("%+v", rows)
	}
	m := withRows(rows)
	if a := m.Activate(false); a.Kind != ActLaunch || a.Entry.Name != "Discord" {
		t.Fatal("two equal names are two rows, Enter starts the selected one")
	}

	low := []match.Result{{Index: 3, Score: 0.5}, {Index: 2, Score: 0.3}, {Index: 0, Score: 0}}
	rows = ProgramRows(ix, low)
	if len(rows) != 2 || !rows[0].Blocked || !rows[1].Blocked {
		t.Fatalf("below PickerMin: %+v", rows)
	}
	m = withRows(rows)
	if a := m.Activate(false); a.Kind != ActNone {
		t.Fatalf("Enter must not start a weak program: %+v", a)
	}
	if a := m.Activate(true); a.Kind != ActReveal {
		t.Fatalf("Shift+Enter still reveals: %+v", a)
	}
}

func TestSearchResultOrderAndEverythingMissing(t *testing.T) {
	programs := []Row{{Entry: index.Entry{Name: "Notepad", Kind: index.KindShortcut, Target: `C:\n.lnk`}}}
	files := &index.Index{Entries: []index.Entry{
		{Name: "notes.txt", Kind: index.KindFile, Target: `E:\a\notes.txt`},
		{Name: "notes.txt", Kind: index.KindFile, Target: `E:\typed\notes.txt`},
		{Name: "notes", Kind: index.KindFolder, Target: `E:\b\notes`},
	}}
	ranked := []match.Result{{Index: 2, Score: 1}, {Index: 0, Score: 0.9}, {Index: 1, Score: 0.9}}
	typed := index.Entry{Name: "notes.txt", Kind: index.KindFile, Target: `E:\Typed\notes.txt`}
	r := SearchResult(7, programs, &typed, files, ranked, nil)
	var got []string
	for _, row := range r.Rows {
		got = append(got, row.Entry.Target)
	}
	want := []string{`C:\n.lnk`, `E:\Typed\notes.txt`, `E:\b\notes`, `E:\a\notes.txt`}
	if strings.Join(got, "|") != strings.Join(want, "|") || r.Gen != 7 {
		t.Fatalf("got %v", got)
	}

	errEv := errors.New("file search needs Everything (https://www.voidtools.com)")
	r = SearchResult(8, programs, nil, nil, nil, errEv)
	if len(r.Rows) != 1 || r.Note != errEv.Error() {
		t.Fatalf("programs must stay, note must name Everything: %+v", r)
	}
}

func TestFileEnterOpensEvenWhenProgramsWeak(t *testing.T) {
	rows := []Row{
		{Entry: index.Entry{Name: "Paint", Kind: index.KindShortcut}, Blocked: true},
		{Entry: index.Entry{Name: "x.txt", Kind: index.KindFile, Target: `E:\x.txt`}},
	}
	m := withRows(rows)
	m.Move(1)
	if a := m.Activate(false); a.Kind != ActLaunch || a.Entry.Kind != index.KindFile {
		t.Fatalf("%+v", a)
	}
}

func TestLabel(t *testing.T) {
	p := Row{Entry: index.Entry{Name: "Visual Studio Code", Kind: index.KindShortcut, Target: `C:\x\Visual Studio Code.lnk`}}
	if n, d := p.Label(); n != "Visual Studio Code" || d != "" {
		t.Errorf("program %q %q", n, d)
	}
	a := Row{Entry: index.Entry{Name: "notes txt", Kind: index.KindFile, Target: `E:\a\work\notes.txt`}}
	b := Row{Entry: index.Entry{Name: "notes txt", Kind: index.KindFile, Target: `D:\b\work\notes.txt`}}
	na, da := a.Label()
	nb, db := b.Label()
	if na != "notes.txt" || nb != "notes.txt" || da == db || da != `E:\a\work` {
		t.Errorf("files %q %q / %q %q", na, da, nb, db)
	}
	f := Row{Entry: index.Entry{Name: "work", Kind: index.KindFolder, Target: `E:\a\work`}}
	if n, d := f.Label(); n != "work" || d != `E:\a` {
		t.Errorf("folder %q %q", n, d)
	}
}

func TestTypedPath(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if e, ok := TypedPath(txt); !ok || e.Kind != index.KindFile || e.Target != txt {
		t.Errorf("file %+v %v", e, ok)
	}
	if e, ok := TypedPath(dir); !ok || e.Kind != index.KindFolder || e.Target != dir {
		t.Errorf("folder %+v %v", e, ok)
	}
	if _, ok := TypedPath("discord"); ok {
		t.Error("bare word")
	}
	if _, ok := TypedPath(filepath.Join(dir, "missing.txt")); ok {
		t.Error("missing")
	}
}

func TestRecentRows(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ix := &index.Index{Entries: []index.Entry{
		{Name: "Discord", Kind: index.KindShortcut, Target: `C:\lnk\Discord.lnk`},
		{Name: "Calculator", Kind: index.KindStore, Target: "Calc!App", AUMID: "Calc!App"},
	}}
	ids := []string{
		"path:" + strings.ToLower(file),
		"aumid:calc!app",
		"path:" + strings.ToLower(filepath.Join(dir, "gone.txt")),
		`path:c:\lnk\discord.lnk`,
		"path:" + strings.ToLower(dir),
		"aumid:removed!app",
	}
	rows := RecentRows(ids, ix, os.Stat)
	var got []string
	for _, r := range rows {
		got = append(got, string(r.Entry.Kind)+":"+r.Entry.Name)
	}
	want := []string{"file:a.txt", "store:Calculator", "shortcut:Discord", "folder:" + rows[3].Entry.Name}
	if strings.Join(got, "|") != strings.Join(want, "|") || rows[3].Entry.Kind != index.KindFolder {
		t.Fatalf("got %v", got)
	}
	for _, r := range rows {
		if r.Blocked {
			t.Error("recent rows start with Enter")
		}
	}

	many := make([]string, 30)
	for i := range many {
		many[i] = `path:c:\lnk\discord.lnk`
	}
	if n := len(RecentRows(many, ix, os.Stat)); n != recentRows {
		t.Errorf("at most %d, got %d", recentRows, n)
	}
}
