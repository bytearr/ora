// Package gui is the search window. model.go is the state and its rules,
// without Win32; the window and the hotkey hook are in *_windows.go.
package gui

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ora/core/discover"
	"ora/core/index"
	"ora/core/match"
)

const (
	visibleRows  = 5  // hits before Show more
	expandedRows = 12 // hits on screen after Show more, the rest scrolls
	recentRows   = 20
	debounce     = 1 * time.Millisecond // SetTimer raises it to USER_TIMER_MINIMUM, 10 ms
)

// Row is one hit.
type Row struct {
	Entry index.Entry
	// Blocked rows are shown but Enter does not start them: programs when
	// the best program score is below match.PickerMin.
	Blocked bool
}

func (r Row) IsPath() bool {
	return r.Entry.Kind == index.KindFile || r.Entry.Kind == index.KindFolder
}

// Label is the text of the row. A file or folder shows its name and the
// folder it is in, so two equal names stay apart.
func (r Row) Label() (name, dir string) {
	if !r.IsPath() {
		return r.Entry.Name, ""
	}
	return filepath.Base(r.Entry.Target), filepath.Dir(r.Entry.Target)
}

// wordStart is where Ctrl+Backspace deletes back to from caret: past the
// spaces before it, then past one run of word characters or of other
// characters, so `C:\Users\foo` loses `foo`, then `\`, then `Users`.
func wordStart(text []uint16, caret int) int {
	isWord := func(u uint16) bool {
		r := rune(u)
		return u >= 0xD800 && u < 0xE000 || unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	i := caret
	for i > 0 && unicode.IsSpace(rune(text[i-1])) {
		i--
	}
	if i == 0 {
		return 0
	}
	word := isWord(text[i-1])
	for i > 0 && !unicode.IsSpace(rune(text[i-1])) && isWord(text[i-1]) == word {
		i--
	}
	return i
}

// Mode is what a query asks for.
type Mode int

const (
	ModeRecent Mode = iota // empty: the recent list
	ModeNone               // one character: nothing
	ModeSearch             // two or more: ranked search after the debounce
)

func QueryMode(q string) Mode {
	switch utf8.RuneCountInString(strings.TrimSpace(q)) {
	case 0:
		return ModeRecent
	case 1:
		return ModeNone
	default:
		return ModeSearch
	}
}

// Result is what the worker delivers for one generation. A search comes in
// two: Partial with the programs (and a typed path) as soon as they are
// ranked, then the full list, whose rows start with the same ones.
type Result struct {
	Gen     uint64
	Rows    []Row
	Note    string
	Partial bool
}

// Item is one line under the query: a row or the Show more button.
type Item struct {
	Row  int // index into Model.Rows, -1 for the button
	More bool
}

type ActionKind int

const (
	ActNone ActionKind = iota
	ActExpand
	ActLaunch
	ActReveal
)

type Action struct {
	Kind  ActionKind
	Entry index.Entry
}

type Model struct {
	Query    string
	Gen      uint64
	Rows     []Row
	Note     string
	Sel      int
	Expanded bool
	Top      int // first item on screen when expanded
	// Stale: Rows belong to an older query until the next Apply.
	Stale bool

	pending, pendingReveal bool // Enter pressed while Stale
	partial                bool // Rows are a Partial result of Gen
}

// SetQuery starts a new generation: results of older ones are dropped and
// the list collapses. keep leaves the old rows on screen until the new
// result arrives, so typing does not blank the list; the caller passes it
// only when a result for q will come.
func (m *Model) SetQuery(q string, keep bool) Mode {
	m.Query = q
	m.Gen++
	if !keep {
		m.Rows, m.Note = nil, ""
	}
	m.Stale, m.pending, m.partial = keep, false, false
	m.Sel, m.Top, m.Expanded = 0, 0, false
	return QueryMode(q)
}

// Apply takes a worker result. A result of an older generation is
// discarded and Apply returns false. An empty Partial is skipped too: the
// old rows stay until there is something to show. The full result after a
// shown Partial only appends rows, so the selection and scroll stay.
func (m *Model) Apply(r Result) bool {
	if r.Gen != m.Gen || r.Partial && len(r.Rows) == 0 {
		return false
	}
	extends := m.partial && !r.Partial
	m.Rows, m.Note, m.Stale, m.partial = r.Rows, r.Note, false, r.Partial
	if !extends {
		m.Sel, m.Top, m.Expanded = 0, 0, false
	}
	m.Sel = max(0, min(m.Sel, len(m.Items())-1))
	return true
}

// TakePending is the Enter that was pressed on stale rows, now applied
// to the fresh ones. ActNone if there was none.
func (m *Model) TakePending() Action {
	if !m.pending || m.Stale {
		return Action{}
	}
	m.pending = false
	return m.activate(m.pendingReveal)
}

// Items is the list Move walks: before Show more the first visibleRows
// hits and the button, after it every hit.
func (m *Model) Items() []Item {
	n := len(m.Rows)
	if !m.Expanded && n > visibleRows {
		n = visibleRows
	}
	items := make([]Item, 0, n+1)
	for i := 0; i < n; i++ {
		items = append(items, Item{Row: i})
	}
	if !m.Expanded && len(m.Rows) > visibleRows {
		items = append(items, Item{Row: -1, More: true})
	}
	return items
}

// Visible is the part of Items on screen; first is the index of its first
// item in Items.
func (m *Model) Visible() (first int, items []Item) {
	all := m.Items()
	if !m.Expanded {
		return 0, all
	}
	end := min(m.Top+expandedRows, len(all))
	return m.Top, all[m.Top:end]
}

// Move shifts the selection within Items and keeps it on screen.
func (m *Model) Move(d int) {
	n := len(m.Items())
	if n == 0 {
		return
	}
	m.Sel = max(0, min(n-1, m.Sel+d))
	m.follow()
}

// Scroll moves the expanded list without moving the selection.
func (m *Model) Scroll(d int) {
	if !m.Expanded {
		return
	}
	m.Top = max(0, min(max(0, len(m.Rows)-expandedRows), m.Top+d))
}

func (m *Model) follow() {
	if !m.Expanded {
		m.Top = 0
		return
	}
	if m.Sel < m.Top {
		m.Top = m.Sel
	}
	if m.Sel >= m.Top+expandedRows {
		m.Top = m.Sel - expandedRows + 1
	}
}

// Activate is Enter (reveal: Shift+Enter) on the selected item. It never
// starts anything itself; it returns the entry the window should start.
// On stale rows it waits for the result of the typed query: the old top
// hit is not what was asked for.
func (m *Model) Activate(reveal bool) Action {
	if m.Stale {
		m.pending, m.pendingReveal = true, reveal
		return Action{}
	}
	return m.activate(reveal)
}

func (m *Model) activate(reveal bool) Action {
	items := m.Items()
	if m.Sel < 0 || m.Sel >= len(items) {
		return Action{}
	}
	it := items[m.Sel]
	if it.More {
		m.Expanded = true
		m.follow()
		return Action{Kind: ActExpand}
	}
	r := m.Rows[it.Row]
	switch {
	case reveal:
		return Action{Kind: ActReveal, Entry: r.Entry}
	case r.Blocked:
		return Action{}
	default:
		return Action{Kind: ActLaunch, Entry: r.Entry}
	}
}

// Click selects the i-th visible item and activates it, stale or not:
// the click aims at the row on screen.
func (m *Model) Click(i int, reveal bool) Action {
	first, items := m.Visible()
	if i < 0 || i >= len(items) {
		return Action{}
	}
	m.Sel = first + i
	return m.activate(reveal)
}

// ProgramRows turns ranked index results into rows. Every hit at or above
// match.PickerMin is kept. When the best is below, the closest
// match.NoMatchRows are shown and Enter does not start them.
func ProgramRows(ix *index.Index, rs []match.Result) []Row {
	var rows []Row
	if len(rs) == 0 || ix == nil {
		return nil
	}
	if rs[0].Score >= match.PickerMin {
		for _, r := range rs {
			if r.Score < match.PickerMin {
				break
			}
			rows = append(rows, Row{Entry: ix.Entries[r.Index]})
		}
		return rows
	}
	for _, r := range rs {
		if len(rows) == match.NoMatchRows || r.Score <= 0 {
			break
		}
		rows = append(rows, Row{Entry: ix.Entries[r.Index], Blocked: true})
	}
	return rows
}

// DropMissing removes programs found by Everything whose file is gone. The
// index keeps those entries until `ora index`, e.g. after an app update
// moved its folder.
func DropMissing(rows []Row, stat func(string) (os.FileInfo, error)) []Row {
	out := rows[:0:0]
	for _, r := range rows {
		if r.Entry.Kind == index.KindPortable {
			if _, err := stat(r.Entry.Target); err != nil {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// PathRows turns ranked file search results into rows, skipping paths
// already listed.
func PathRows(ix *index.Index, rs []match.Result, skip map[string]bool) []Row {
	if ix == nil {
		return nil
	}
	rows := make([]Row, 0, len(rs))
	for _, r := range rs {
		e := ix.Entries[r.Index]
		if skip[strings.ToLower(e.Target)] {
			continue
		}
		rows = append(rows, Row{Entry: e})
	}
	return rows
}

// SearchResult puts programs before files and folders. A typed path is the
// first file row. A failed file search keeps the programs and becomes the
// note.
func SearchResult(gen uint64, programs []Row, typed *index.Entry, files *index.Index, ranked []match.Result, filesErr error) Result {
	r := Result{Gen: gen, Rows: append([]Row(nil), programs...)}
	skip := map[string]bool{}
	if typed != nil {
		r.Rows = append(r.Rows, Row{Entry: *typed})
		skip[strings.ToLower(typed.Target)] = true
	}
	if filesErr != nil {
		r.Note = filesErr.Error()
		return r
	}
	r.Rows = append(r.Rows, PathRows(files, ranked, skip)...)
	return r
}

// TypedPath is the entry for a query that names an existing file or
// folder. A bare word without separator or dot stays a search.
func TypedPath(q string) (index.Entry, bool) {
	q = strings.TrimSpace(q)
	if q == "" || (!strings.ContainsAny(q, `\/`) && !strings.Contains(q, ".")) {
		return index.Entry{}, false
	}
	st, err := os.Stat(q)
	if err != nil {
		return index.Entry{}, false
	}
	if abs, err := filepath.Abs(q); err == nil {
		q = abs
	}
	if st.IsDir() {
		return discover.DirEntry(q), true
	}
	return discover.FileEntry(q), true
}

// RecentRows maps recent IDs to rows in recent order, at most recentRows.
// An index ID is a program row; a path ID that is not in the index is a
// file or folder row if the path still exists.
func RecentRows(ids []string, ix *index.Index, stat func(string) (os.FileInfo, error)) []Row {
	byID := map[string]index.Entry{}
	if ix != nil {
		for _, e := range ix.Entries {
			if _, ok := byID[e.ID()]; !ok {
				byID[e.ID()] = e
			}
		}
	}
	var rows []Row
	for _, id := range ids {
		if len(rows) == recentRows {
			break
		}
		if e, ok := byID[id]; ok {
			rows = append(rows, Row{Entry: e})
			continue
		}
		p, ok := strings.CutPrefix(id, "path:")
		if !ok || p == "" {
			continue
		}
		st, err := stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			rows = append(rows, Row{Entry: discover.DirEntry(p)})
		} else {
			rows = append(rows, Row{Entry: discover.FileEntry(p)})
		}
	}
	return rows
}
