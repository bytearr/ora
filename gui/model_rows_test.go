package gui

import (
	"fmt"
	"testing"

	"ora/core/index"
	"ora/core/match"
)

func TestFileNotBuriedUnderWeakPrograms(t *testing.T) {
	var programs []Row
	for i := range 30 {
		programs = append(programs, Row{Entry: index.Entry{Name: fmt.Sprintf("App %02d", i), Kind: index.KindShortcut, Target: fmt.Sprintf("/apps/%02d.desktop", i)}})
	}
	file := index.Entry{Name: "ora-setup.sh", Kind: index.KindFile, Target: "/home/kali/ora-setup.sh"}
	files := &index.Index{Entries: []index.Entry{file}}
	ranked := []match.Result{{Index: 0, Score: match.ScoreExact}}

	var m Model
	m.SetQuery("ora-setup", false)
	partial := SearchResult(m.Gen, programs, nil, nil, nil, nil)
	partial.Partial = true
	if len(partial.Rows) != match.PickerRows {
		t.Fatalf("partial: %d rows, want the top %d programs", len(partial.Rows), match.PickerRows)
	}
	m.Apply(partial)
	m.Move(2)

	full := SearchResult(m.Gen, programs, nil, files, ranked, nil)
	for i, r := range partial.Rows {
		if full.Rows[i] != r {
			t.Fatalf("row %d changed: the full result must only append", i)
		}
	}
	if len(full.Rows) != len(programs)+1 || full.Rows[match.PickerRows].Entry != file {
		t.Fatalf("file must follow the top %d programs: %d rows", match.PickerRows, len(full.Rows))
	}
	if last := full.Rows[len(full.Rows)-1].Entry; last != programs[len(programs)-1].Entry {
		t.Fatalf("weaker programs follow the files, last row %q", last.Name)
	}
	m.Apply(full)
	if m.Sel != 2 {
		t.Fatalf("selection moved to %d", m.Sel)
	}

	m.Move(visibleRows)
	if a := m.Activate(false); a.Kind != ActExpand {
		t.Fatalf("Show more: %+v", a)
	}
	_, vis := m.Visible()
	if len(vis) != expandedRows {
		t.Fatalf("expanded: %d rows on screen", len(vis))
	}
	found := false
	for _, it := range vis {
		found = found || m.Rows[it.Row].Entry == file
	}
	if !found {
		t.Fatal("the file must be on the first expanded page")
	}

	failed := SearchResult(m.Gen, programs, nil, nil, nil, fmt.Errorf("index down"))
	if len(failed.Rows) != len(programs) || failed.Note == "" {
		t.Fatalf("a failed file search keeps every program: %d rows", len(failed.Rows))
	}
}
