//go:build !windows

package gui

import (
	"testing"

	"ora/core/index"
)

func rowsOf(names ...string) []Row {
	var rs []Row
	for _, n := range names {
		rs = append(rs, Row{Entry: index.Entry{Name: n, Kind: index.KindDesktop, Target: "/a/" + n + ".desktop"}})
	}
	return rs
}

func TestLinesEmpty(t *testing.T) {
	var m Model
	ls, h := lines(&m, nil)
	if ls != nil || h != 2*padPx+boxPx {
		t.Fatalf("empty: %v %d", ls, h)
	}
}

func TestLinesGroupsAndHeight(t *testing.T) {
	var m Model
	m.SetQuery("no", true)
	file := Row{Entry: index.Entry{Name: "notes.txt", Kind: index.KindFile, Target: "/home/u/notes.txt"}}
	m.Apply(Result{Gen: m.Gen, Rows: append(rowsOf("Notepad", "Nomacs"), file)})
	ls, h := lines(&m, []string{"a note"})
	var kinds []lineKind
	var texts []string
	for _, l := range ls {
		kinds = append(kinds, l.kind)
		if l.kind == lineHeader {
			texts = append(texts, l.text)
		}
	}
	want := []lineKind{lineHeader, lineRow, lineRow, lineHeader, lineRow, lineNote}
	if len(kinds) != len(want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds %v, want %v", kinds, want)
		}
	}
	if texts[0] != "Apps" || texts[1] != "Files & folders" {
		t.Fatalf("headers %v", texts)
	}
	wantH := 2*padPx + boxPx + 2*listGapPx + 2*headerPx + 3*rowPx + notePx
	if h != wantH {
		t.Fatalf("height %d, want %d", h, wantH)
	}
	if ls[0].y != 2*padPx+boxPx+listGapPx {
		t.Fatalf("first line at %d", ls[0].y)
	}
}

func TestLinesRecentAndMore(t *testing.T) {
	var m Model
	m.SetQuery("", false)
	m.Apply(Result{Gen: m.Gen, Rows: rowsOf("a", "b", "c", "d", "e", "f", "g")})
	ls, h := lines(&m, nil)
	if ls[0].kind != lineHeader || ls[0].text != "Recent" {
		t.Fatalf("first line %+v", ls[0])
	}
	if last := ls[len(ls)-1]; last.kind != lineMore {
		t.Fatalf("last line %+v", last)
	}
	if h != collapsedHeight() {
		t.Fatalf("a full first page is %d high, collapsedHeight says %d", h, collapsedHeight())
	}
}

func TestTopMargin(t *testing.T) {
	c := collapsedHeight()
	if got := topMargin(1080, 100, false); got != (1080-c)/2 {
		t.Fatalf("collapsed: %d", got)
	}
	if got := topMargin(1080, 900, true); got != 90 {
		t.Fatalf("expanded: %d", got)
	}
	if got := topMargin(1080, 1000, false); got != 80 {
		t.Fatalf("a tall list stays on the monitor: %d", got)
	}
	if got := topMargin(300, 1000, true); got != 0 {
		t.Fatalf("never above the monitor: %d", got)
	}
}

func TestFitRows(t *testing.T) {
	var m Model
	m.SetQuery("", false)
	var names []string
	for i := range 30 {
		names = append(names, string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	m.Apply(Result{Gen: m.Gen, Rows: rowsOf(names...)})
	m.Move(visibleRows)
	m.Activate(false) // Show more
	m.Move(20)
	_, full := lines(&m, nil)

	if fitRows(&m, nil, 1003) {
		t.Fatal("a tall enough screen keeps the rows")
	}
	if _, h := lines(&m, nil); h != full {
		t.Fatalf("height changed on a tall screen: %d, was %d", h, full)
	}

	if !fitRows(&m, nil, 500) {
		t.Fatal("a short screen must lower the rows")
	}
	_, h := lines(&m, nil)
	_, exp := m.RowLimits()
	if h > 500 || exp >= expandedRows {
		t.Fatalf("height %d with %d rows on 500 px", h, exp)
	}
	if first, vis := m.Visible(); m.Sel < first || m.Sel >= first+len(vis) {
		t.Fatalf("selection %d off screen (%d..%d)", m.Sel, first, first+len(vis)-1)
	}
	m.Scroll(100)
	if first, vis := m.Visible(); first+len(vis) != len(m.Rows) {
		t.Fatal("the rest must still scroll in")
	}

	m.SetQuery("", false)
	m.Apply(Result{Gen: m.Gen, Rows: rowsOf(names...)})
	fitRows(&m, nil, 300)
	vis, _ := m.RowLimits()
	if _, h := lines(&m, nil); h > 300 || vis >= visibleRows {
		t.Fatalf("collapsed: height %d with %d rows on 300 px", h, vis)
	}
	fitRows(&m, nil, 2000)
	if v, e := m.RowLimits(); v != visibleRows || e != expandedRows {
		t.Fatalf("a tall screen restores the maximum: %d %d", v, e)
	}
}

func TestRowText(t *testing.T) {
	name, sub, path := rowText(Row{Entry: index.Entry{Name: "Firefox", Kind: index.KindDesktop}})
	if name != "Firefox" || sub != "App" || path {
		t.Fatalf("app: %q %q %v", name, sub, path)
	}
	name, sub, path = rowText(Row{Entry: index.Entry{Name: "notes.txt", Kind: index.KindFile, Target: "/home/u/docs/notes.txt"}})
	if name != "notes.txt" || sub != "/home/u/docs" || !path {
		t.Fatalf("file: %q %q %v", name, sub, path)
	}
}
