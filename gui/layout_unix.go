//go:build !windows

package gui

// palette: bg window, field search box, sel selected row, line separator
// and border, dim secondary text, accent the fallback when the desktop has
// none. The same colors as the Windows window.
type palette struct{ bg, field, sel, line, text, dim, accent string }

var palettes = map[string]palette{
	"dark":  {bg: "#202020", field: "#2d2d2d", sel: "#333333", line: "#3a3a3a", text: "#ffffff", dim: "#9e9e9e", accent: "#4cc2ff"},
	"light": {bg: "#f9f9f9", field: "#ffffff", sel: "#eaeaea", line: "#e0e0e0", text: "#1a1a1a", dim: "#616161", accent: "#005fb8"},
}

// Sizes in logical pixels, as on Windows; GTK scales them.
const (
	winWidth  = 720
	padPx     = 12 // around the search box
	boxPx     = 44 // search box height
	listGapPx = 8  // separator to first line, last line to bottom
	headerPx  = 30
	rowPx     = 52
	morePx    = 40
	notePx    = 32
	insetPx   = 8 // row highlight to window edge
	radiusPx  = 6
	iconPx    = 32
	fontPx    = 15
	smallPx   = 12
	wheelRows = 3
)

type lineKind int

const (
	lineHeader lineKind = iota
	lineRow
	lineMore
	lineNote
)

// line is one band under the search box. item indexes Model.Visible.
type line struct {
	kind lineKind
	item int
	text string
	y, h int
}

// group is the section header above a row.
func group(m *Model, r Row) string {
	switch {
	case QueryMode(m.Query) == ModeRecent:
		return "Recent"
	case r.IsPath():
		return "Files & folders"
	default:
		return "Apps"
	}
}

// lines lays out everything under the search box; the widgets and the
// window height both use it. A header starts each group on screen.
func lines(m *Model, notes []string) (ls []line, height int) {
	y := 2*padPx + boxPx
	_, items := m.Visible()
	if len(items)+len(notes) == 0 {
		return nil, y
	}
	y += listGapPx
	add := func(l line, h int) {
		l.y, l.h = y, h
		ls = append(ls, l)
		y += h
	}
	g := ""
	for i, it := range items {
		if it.More {
			add(line{kind: lineMore, item: i}, morePx)
			continue
		}
		if ng := group(m, m.Rows[it.Row]); ng != g {
			g = ng
			add(line{kind: lineHeader, text: g}, headerPx)
		}
		add(line{kind: lineRow, item: i}, rowPx)
	}
	for _, n := range notes {
		add(line{kind: lineNote, text: n}, notePx)
	}
	return ls, y + listGapPx
}

// collapsedHeight is the window with a full first page. The window hangs
// from where that height is centered, so typing does not move the box.
func collapsedHeight() int {
	return 2*padPx + boxPx + 2*listGapPx + headerPx + visibleRows*rowPx + morePx
}

// fitRows lowers the rows m shows, in the state it is in, until the card is
// at most avail high; the rest scrolls. The constants stay the maximum.
// It reports whether the limits changed.
func fitRows(m *Model, notes []string, avail int) bool {
	v0, e0 := m.RowLimits()
	v, e := visibleRows, expandedRows
	for {
		m.SetRowLimits(v, e)
		if _, h := lines(m, notes); h <= avail {
			break
		}
		if m.Expanded && e > 1 {
			e--
		} else if !m.Expanded && v > 1 {
			v--
		} else {
			break
		}
	}
	v1, e1 := m.RowLimits()
	return v1 != v0 || e1 != e0
}

// topMargin is the distance of the window from the top of a monitor of
// height monH: a collapsed list hangs from a fixed top, an expanded one is
// centered; both stay on the monitor.
func topMargin(monH, h int, expanded bool) int {
	y := (monH - collapsedHeight()) / 2
	if expanded {
		y = (monH - h) / 2
	}
	return max(0, min(y, monH-h))
}

// rowText is the name and the second line of a row: the folder of a file
// or folder, else what kind of program it is. path is set when the second
// line is a folder, which is shortened in the middle.
func rowText(r Row) (name, sub string, path bool) {
	name, dir := r.Label()
	switch {
	case dir != "":
		return name, dir, true
	default:
		return name, "App", false
	}
}
