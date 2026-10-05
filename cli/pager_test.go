package cli

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(k tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: k} }

func TestPager(t *testing.T) {
	rows := make([]string, 45)
	for i := range rows {
		rows[i] = fmt.Sprintf("row%02d", i)
	}
	var m tea.Model = pager{header: "NAME", rows: rows}
	view := func() string { return m.View() }

	if v := view(); !strings.Contains(v, "row00") || strings.Contains(v, "row20") || !strings.Contains(v, "Page 1/3") {
		t.Fatalf("page 1:\n%s", v)
	}
	m, _ = m.Update(key(tea.KeyLeft))
	if !strings.Contains(view(), "Page 1/3") {
		t.Error("left on first page must stay")
	}
	m, _ = m.Update(key(tea.KeyRight))
	if v := view(); !strings.Contains(v, "row20") || strings.Contains(v, "row19") || !strings.Contains(v, "Page 2/3") {
		t.Fatalf("page 2:\n%s", v)
	}
	m, _ = m.Update(key(tea.KeyEnd))
	m, _ = m.Update(key(tea.KeyRight))
	if v := view(); !strings.Contains(v, "row44") || !strings.Contains(v, "Page 3/3") {
		t.Fatalf("last page:\n%s", v)
	}
	if n := strings.Count(view(), "\n"); n != 1+PageSize+1 {
		t.Errorf("short last page must keep height, got %d lines", n)
	}
	m, _ = m.Update(key(tea.KeyHome))
	if !strings.Contains(view(), "Page 1/3") {
		t.Error("home")
	}

	m, _ = m.Update(tea.WindowSizeMsg{Width: 10})
	for _, line := range strings.Split(view(), "\n") {
		if len([]rune(line)) > 10 {
			t.Errorf("line not truncated to width: %q", line)
		}
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q must quit")
	}
}
