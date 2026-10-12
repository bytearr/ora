//go:build linux && cgo && !nogui

package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"ora/core/config"
	"ora/core/discover"
	"ora/core/engine"
	"ora/core/index"
)

// debounceMs is the pause after a key press before searching; Windows
// waits its minimal timer, 10 ms.
const debounceMs = 10

// window is the GTK side: the same model and pipeline as on Windows,
// shown as widgets styled like the Windows window.
type window struct {
	app   *gtk.Application
	win   *gtk.ApplicationWindow
	card  *gtk.Box
	entry *gtk.Entry
	sep   *gtk.Box
	list  *gtk.Box

	log         *logger
	eng         *engine.Engine
	settings    config.Settings
	hk          config.Hotkey
	s           session
	m           Model
	ix          *index.Index
	ixErr       string
	status      []string
	icons       map[string]gio.Iconner
	layer       bool // a layer-shell overlay, placed by us; else a plain window
	noIcons     bool // no SVG loader: GTK would recurse looking for image-missing
	startHidden bool
	settingText bool
	busy        bool
	debounce    coreglib.SourceHandle
	scrollAcc   float64
	fatal       error
}

// activate is GApplication's activate: the first one starts the window,
// every later one (`ora` again, a desktop shortcut) toggles it.
func (w *window) activate() {
	if w.win != nil {
		w.toggle()
		return
	}
	w.app.Hold()
	if err := w.start(); err != nil {
		w.log.printf("start failed: %v", err)
		w.fatal = err
		w.app.Release()
		w.app.Quit()
	}
}

func (w *window) start() error {
	var warns []string
	warn := func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		warns = append(warns, s)
		w.log.printf("warning: %s", s)
	}
	if path, created, err := config.EnsureConfigFile(); err != nil {
		warn("config: %v", err)
	} else if created {
		w.log.printf("created %s", path)
	}
	eng, err := engine.Open(w.log.printf)
	if err != nil {
		return err
	}
	eng.LiveFiles = true
	w.eng = eng
	w.settings, w.hk = config.CheckSettings(eng.Cfg.Settings, warn)
	if err := syncAutostart(w.settings.Autostart); err != nil {
		warn("autostart: %v", err)
	}
	if err := syncAppEntry(); err != nil {
		w.log.printf("app entry: %v", err)
	}
	w.status = warns
	w.icons = map[string]gio.Iconner{}

	w.build()
	w.s = session{eng: eng, logf: w.log.printf, post: func(f func()) { coreglib.IdleAdd(f) }, onResult: w.onResult}
	w.s.start()
	w.s.loadIndex(w.onIndex)
	go w.bindHotkey()
	if !w.startHidden {
		w.show()
	}
	return nil
}

// bindHotkey asks the desktop for settings.hotkey through the global
// shortcuts portal. Without one, a desktop shortcut that runs `ora` does
// the same: a second `ora` toggles the window.
func (w *window) bindHotkey() {
	trigger := shortcutTrigger(w.hk)
	bound, err := bindShortcut(trigger, func(tok string) {
		coreglib.IdleAdd(func() {
			if tok != "" {
				w.win.SetStartupID(tok)
			}
			w.toggle()
		})
	})
	if err != nil {
		w.log.printf("hotkey: %v", err)
		msg := "hotkey: " + err.Error() + "; bind a desktop shortcut to the command ora"
		if errors.Is(err, errNoPortal) {
			msg = "No global shortcut portal: bind a desktop shortcut to the command ora"
		}
		coreglib.IdleAdd(func() {
			w.status = append(w.status, msg)
			w.render()
		})
		return
	}
	w.log.printf("hotkey bound: %q (asked for %q)", bound, trigger)
}

func (w *window) build() {
	w.win = gtk.NewApplicationWindow(w.app)
	w.win.SetTitle("ora")
	w.win.SetDecorated(false)
	w.win.AddCSSClass("ora-window")
	w.layer = layerSupported()
	if !w.layer {
		w.log.printf("no layer shell: using a normal window")
	}
	if w.layer {
		layerInit(&w.win.Window)
	} else {
		w.win.SetResizable(false) // sized by its content
		w.win.SetDefaultSize(winWidth, -1)
	}

	w.card = gtk.NewBox(gtk.OrientationVertical, 0)
	w.card.AddCSSClass("ora")
	w.card.SetSizeRequest(winWidth, -1)
	w.card.SetHAlign(gtk.AlignCenter)
	w.card.SetVAlign(gtk.AlignStart)
	w.card.SetOverflow(gtk.OverflowHidden)
	if w.layer {
		w.card.AddCSSClass("floating")
	}

	w.entry = gtk.NewEntry()
	w.entry.AddCSSClass("search")
	w.entry.SetPlaceholderText("Type to search")
	w.noIcons = !svgSupported()
	if w.noIcons {
		w.log.printf("warning: gdk-pixbuf has no SVG loader (librsvg); showing no icons")
	} else {
		w.entry.SetIconFromIconName(gtk.EntryIconPrimary, "system-search-symbolic")
	}
	w.entry.SetSizeRequest(-1, boxPx)
	setMargins(&w.entry.Widget, padPx, padPx, padPx, padPx)
	w.entry.ConnectChanged(w.onEdit)
	w.card.Append(w.entry)

	w.sep = gtk.NewBox(gtk.OrientationHorizontal, 0)
	w.sep.AddCSSClass("sep")
	w.sep.SetSizeRequest(-1, 1)
	w.card.Append(w.sep)

	w.list = gtk.NewBox(gtk.OrientationVertical, 0)
	w.card.Append(w.list)
	w.win.SetChild(w.card)

	keys := gtk.NewEventControllerKey()
	keys.SetPropagationPhase(gtk.PhaseCapture)
	keys.ConnectKeyPressed(w.onKey)
	w.win.AddController(keys)

	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical | gtk.EventControllerScrollDiscrete)
	scroll.ConnectScroll(func(_, dy float64) bool {
		w.scrollAcc += dy
		steps := int(w.scrollAcc)
		if steps == 0 {
			return true
		}
		w.scrollAcc -= float64(steps)
		w.m.Scroll(steps * wheelRows)
		w.render()
		return true
	})
	w.card.AddController(scroll)

	if w.layer {
		// The overlay covers the monitor: a click beside the card hides it.
		outside := gtk.NewGestureClick()
		outside.ConnectPressed(func(_ int, x, y float64) {
			if !w.inCard(x, y) {
				coreglib.IdleAdd(w.hide)
			}
		})
		w.win.AddController(outside)
		w.win.NotifyProperty("default-height", w.place)
	} else {
		// A plain window hides when it loses the focus, as on Windows.
		w.win.NotifyProperty("is-active", func() {
			if w.win.Visible() && !w.win.IsActive() {
				coreglib.TimeoutAdd(150, func() bool {
					if w.win.Visible() && !w.win.IsActive() {
						w.hide()
					}
					return false
				})
			}
		})
	}
	w.win.ConnectCloseRequest(func() bool {
		w.hide() // Ctrl+Q quits; closing must not drop the hotkey
		return true
	})
	w.win.ConnectMap(func() { coreglib.IdleAdd(w.place) })
	w.loadCSS()
}

func (w *window) loadCSS() {
	p, ok := palettes[w.settings.Theme]
	if !ok {
		p = palettes[config.ThemeDark]
	}
	if a := accentColor(); a != "" {
		p.accent = a
	}
	css := gtk.NewCSSProvider()
	css.LoadFromData(styleSheet(p))
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), css, gtk.STYLE_PROVIDER_PRIORITY_USER)
}

// styleSheet draws the Windows window with CSS. The selectors are
// specific enough to win over the GTK theme.
func styleSheet(p palette) string {
	return strings.NewReplacer(
		"$bg", p.bg, "$field", p.field, "$sel", p.sel, "$line", p.line,
		"$text", p.text, "$dim", p.dim, "$accent", p.accent,
		"${font}", fmt.Sprint(fontPx), "${small}", fmt.Sprint(smallPx), "${radius}", fmt.Sprint(radiusPx),
	).Replace(`
window.ora-window { background: transparent; box-shadow: none; }
window.ora-window .ora {
	background: $bg; color: $text; border: 1px solid $line; border-radius: 8px;
	font-size: ${font}px; font-weight: 400;
}
window.ora-window .ora.floating { box-shadow: 0 8px 28px rgba(0,0,0,0.35); }
window.ora-window entry.search {
	background: $field; color: $text; caret-color: $text;
	border: none; outline: none; border-radius: ${radius}px; min-height: 0;
	box-shadow: inset 0 -2px $accent; padding: 0 12px 0 14px; font-size: ${font}px;
}
window.ora-window entry.search > text > placeholder { color: $dim; opacity: 1; }
window.ora-window entry.search > image { color: $dim; margin-right: 6px; }
window.ora-window entry.search > text > selection { background: alpha($accent, 0.35); color: $text; }
window.ora-window .sep { background: $line; }
window.ora-window .row { border-radius: ${radius}px; }
window.ora-window .row.sel { background: $sel; }
window.ora-window .row .bar { border-radius: 2px; background: transparent; }
window.ora-window .row.sel .bar { background: $accent; }
window.ora-window .name { color: $text; font-size: ${font}px; }
window.ora-window .name.blocked, window.ora-window .sub, window.ora-window .note { color: $dim; }
window.ora-window .sub, window.ora-window .note { font-size: ${small}px; }
window.ora-window .header { color: $dim; font-size: ${small}px; font-weight: 600; }
window.ora-window .more { color: $accent; }
`)
}

func setMargins(wd *gtk.Widget, top, end, bottom, start int) {
	wd.SetMarginTop(top)
	wd.SetMarginEnd(end)
	wd.SetMarginBottom(bottom)
	wd.SetMarginStart(start)
}

func (w *window) inCard(x, y float64) bool {
	picked := w.win.Pick(x, y, gtk.PickDefault)
	if picked == nil {
		return false
	}
	return coreglib.InternObject(picked).Native() != coreglib.InternObject(w.win).Native()
}

func (w *window) notes() []string {
	var n []string
	switch {
	case w.ixErr != "":
		n = append(n, w.ixErr)
	case w.ix == nil:
		n = append(n, "index loading")
	}
	n = append(n, w.status...)
	if w.m.Note != "" {
		n = append(n, w.m.Note)
	}
	return n
}

// render rebuilds the list under the search box from the model.
func (w *window) render() {
	for c := w.list.FirstChild(); c != nil; c = w.list.FirstChild() {
		w.list.Remove(c)
	}
	ls, _ := lines(&w.m, w.notes())
	w.sep.SetVisible(len(ls) > 0)
	if len(ls) > 0 {
		w.list.SetMarginTop(listGapPx - 1) // the separator is the first pixel
		w.list.SetMarginBottom(listGapPx)
	} else {
		w.list.SetMarginTop(0)
		w.list.SetMarginBottom(0)
	}
	first, items := w.m.Visible()
	for _, l := range ls {
		w.list.Append(w.lineWidget(l, first, items))
	}
	w.place()
}

func (w *window) lineWidget(l line, first int, items []Item) gtk.Widgetter {
	label := func(text, class string, ell pango.EllipsizeMode) *gtk.Label {
		lb := gtk.NewLabel(text)
		lb.AddCSSClass(class)
		lb.SetXAlign(0)
		lb.SetEllipsize(ell)
		lb.SetSingleLineMode(true)
		return lb
	}
	switch l.kind {
	case lineHeader, lineNote:
		class := "header"
		if l.kind == lineNote {
			class = "note"
		}
		lb := label(l.text, class, pango.EllipsizeEnd)
		lb.SetSizeRequest(-1, l.h)
		setMargins(&lb.Widget, 0, insetPx+12, 0, insetPx+12)
		return lb
	}
	row := gtk.NewBox(gtk.OrientationHorizontal, 0)
	row.AddCSSClass("row")
	row.SetSizeRequest(-1, l.h)
	setMargins(&row.Widget, 0, insetPx, 0, insetPx)
	if first+l.item == w.m.Sel {
		row.AddCSSClass("sel")
	}
	bar := gtk.NewBox(gtk.OrientationHorizontal, 0)
	bar.AddCSSClass("bar")
	bar.SetSizeRequest(3, 16)
	bar.SetVAlign(gtk.AlignCenter)
	row.Append(bar)
	if l.kind == lineMore {
		lb := label("Show more", "more", pango.EllipsizeEnd)
		lb.SetMarginStart(12 - 3)
		row.Append(lb)
	} else {
		r := w.m.Rows[items[l.item].Row]
		img := gtk.NewImage()
		if !w.noIcons {
			img.SetFromGIcon(w.icon(r.Entry))
		}
		img.SetPixelSize(iconPx)
		img.SetMarginStart(12 - 3)
		row.Append(img)
		name, sub, path := rowText(r)
		text := gtk.NewBox(gtk.OrientationVertical, 0)
		text.SetVAlign(gtk.AlignCenter)
		text.SetHExpand(true)
		setMargins(&text.Widget, 0, 12, 0, 12)
		n := label(name, "name", pango.EllipsizeEnd)
		if r.Blocked {
			n.AddCSSClass("blocked")
		}
		text.Append(n)
		ell := pango.EllipsizeEnd
		if path {
			ell = pango.EllipsizeMiddle
		}
		text.Append(label(sub, "sub", ell))
		row.Append(text)
	}
	item := l.item
	click := gtk.NewGestureClick()
	click.ConnectPressed(func(_ int, _, _ float64) {
		reveal := click.CurrentEventState()&gdk.ShiftMask != 0
		coreglib.IdleAdd(func() { // the row is rebuilt; not inside its own handler
			w.do(w.m.Click(item, reveal))
			w.render()
		})
	})
	row.AddController(click)
	return row
}

// icon is the GIcon for an entry: the Icon= of a desktop entry, the
// content type's icon for a file, a folder, else a generic program.
func (w *window) icon(e index.Entry) gio.Iconner {
	key := string(e.Kind) + "\x00" + e.Target
	if ic, ok := w.icons[key]; ok {
		return ic
	}
	ic := loadIcon(e)
	if _, themed := ic.(*gio.ThemedIcon); themed && !gtk.IconThemeGetForDisplay(gdk.DisplayGetDefault()).HasGIcon(ic) {
		ic = gio.NewThemedIcon("application-x-executable")
	}
	w.icons[key] = ic
	return ic
}

// svgSupported reports whether gdk-pixbuf can load SVGs. Icon themes are
// mostly SVG; without the loader GTK 4 finds no icon, not even its
// image-missing fallback, and recurses until the stack overflows.
func svgSupported() bool {
	for _, f := range gdkpixbuf.PixbufGetFormats() {
		for _, mt := range f.MIMETypes() {
			if mt == "image/svg" { // the exact test GTK makes
				return true
			}
		}
	}
	return false
}

func loadIcon(e index.Entry) gio.Iconner {
	switch e.Kind {
	case index.KindDesktop:
		if data, err := os.ReadFile(e.Target); err == nil {
			if name := discover.ParseDesktop(string(data), nil).Icon; name != "" {
				if filepath.IsAbs(name) {
					return gio.NewFileIcon(gio.NewFileForPath(name))
				}
				return gio.NewThemedIcon(name)
			}
		}
	case index.KindFolder:
		return gio.NewThemedIcon("folder")
	case index.KindFile:
		if _, ct := gio.ContentTypeGuess(e.Target, nil); ct != "" {
			if ic := gio.ContentTypeGetIcon(ct); ic != nil {
				return ic
			}
		}
		return gio.NewThemedIcon("text-x-generic")
	}
	return gio.NewThemedIcon("application-x-executable")
}

// place hangs the card where the Windows window would be: a collapsed
// list from a fixed top, an expanded one centered. Only the overlay is
// placed; a plain window is where the desktop puts it.
func (w *window) place() {
	if !w.layer {
		return
	}
	monH := w.win.Height()
	if monH <= 0 {
		return
	}
	_, h := lines(&w.m, w.notes())
	w.card.SetMarginTop(topMargin(monH, h, w.m.Expanded))
}

func (w *window) monitor() *gdk.Monitor {
	if w.settings.Monitor != config.MonitorPrimary {
		return nil // the compositor's focused output, where the pointer works
	}
	ms := gdk.DisplayGetDefault().Monitors()
	if ms.NItems() == 0 {
		return nil
	}
	m, _ := ms.Item(0).Cast().(*gdk.Monitor)
	return m
}

func (w *window) reset() {
	if w.debounce != 0 {
		coreglib.SourceRemove(w.debounce)
		w.debounce = 0
	}
	w.settingText = true
	w.entry.SetText("")
	w.settingText = false
	w.m.SetQuery("", false)
	w.s.latestGen.Store(w.m.Gen)
}

func (w *window) show() {
	if w.win.Visible() {
		w.present()
		return
	}
	w.reset()
	w.requestRecent()
	w.present()
	go refreshAutostart(w.log.printf)
}

// present brings the window up on the configured monitor without
// touching the query.
func (w *window) present() {
	if w.layer && !w.win.Visible() {
		layerMonitor(&w.win.Window, w.monitor())
	}
	w.render()
	w.win.Present()
	w.entry.GrabFocus()
}

func (w *window) hide() {
	w.win.SetVisible(false)
	w.reset()
	w.status = nil
}

// toggle hides the window only when it has the focus; otherwise it shows
// it, as the hotkey does on Windows. A visible overlay holds the keyboard
// exclusively, but GTK never marks a layer surface active.
func (w *window) toggle() {
	if w.win.Visible() && (w.layer || w.win.IsActive()) {
		w.hide()
		return
	}
	w.show()
}

func (w *window) quit() {
	w.win.SetVisible(false)
	w.app.Release()
	w.app.Quit()
}

func (w *window) onEdit() {
	if w.settingText {
		return
	}
	w.status = nil
	q := w.entry.Text()
	mode := w.m.SetQuery(q, QueryMode(q) == ModeSearch && w.ix != nil)
	w.s.latestGen.Store(w.m.Gen)
	if w.debounce != 0 {
		coreglib.SourceRemove(w.debounce)
		w.debounce = 0
	}
	switch mode {
	case ModeRecent:
		w.requestRecent()
	case ModeSearch:
		w.scheduleSearch()
	}
	w.render()
}

func (w *window) scheduleSearch() {
	if w.ix == nil {
		return
	}
	if w.debounce != 0 {
		coreglib.SourceRemove(w.debounce)
	}
	w.debounce = coreglib.TimeoutAdd(debounceMs, func() bool {
		w.debounce = 0
		w.runSearch()
		return false
	})
}

func (w *window) requestRecent() {
	if w.ix != nil {
		w.s.submit(searchReq{gen: w.m.Gen, recent: true, ix: w.ix})
	}
}

func (w *window) runSearch() {
	if w.ix == nil || QueryMode(w.m.Query) != ModeSearch {
		return
	}
	w.s.submit(searchReq{gen: w.m.Gen, query: w.m.Query, ix: w.ix})
}

func (w *window) onIndex(ix *index.Index, err error) {
	if err != nil {
		w.ixErr = "index: " + err.Error()
		w.log.printf("%s", w.ixErr)
	} else {
		w.ix = ix
	}
	switch QueryMode(w.m.Query) {
	case ModeRecent:
		w.requestRecent()
	case ModeSearch:
		w.scheduleSearch()
	}
	w.render()
}

func (w *window) onResult(r Result) {
	if w.m.Apply(r) {
		w.render()
		w.do(w.m.TakePending())
	}
}

func (w *window) do(a Action) {
	switch a.Kind {
	case ActExpand:
		w.render()
	case ActLaunch, ActReveal:
		if w.busy {
			return
		}
		w.busy = true
		ix := w.ix
		go func() {
			defer w.s.recoverWorker("launch")
			e := a.Entry
			var err error
			if a.Kind == ActLaunch {
				if err = w.eng.Launch(ix, e, nil); err == nil {
					w.eng.Remember(e)
				}
			} else {
				if e.Kind == index.KindFolder {
					e.Kind = index.KindFile // Shift+Enter highlights the folder in its parent
				}
				err = w.eng.Reveal(e)
			}
			coreglib.IdleAdd(func() { w.onLaunched(err) })
		}()
	}
}

func (w *window) onLaunched(err error) {
	w.busy = false
	if err == nil {
		w.hide()
		return
	}
	w.log.printf("%v", err)
	w.status = []string{err.Error()}
	if !w.win.Visible() {
		w.present()
	}
	w.render()
}

func (w *window) onKey(keyval, _ uint, state gdk.ModifierType) bool {
	ctrl := state&gdk.ControlMask != 0
	switch keyval {
	case gdk.KEY_Up, gdk.KEY_KP_Up:
		w.m.Move(-1)
	case gdk.KEY_Down, gdk.KEY_KP_Down:
		w.m.Move(1)
	case gdk.KEY_Return, gdk.KEY_KP_Enter, gdk.KEY_ISO_Enter:
		w.do(w.m.Activate(state&gdk.ShiftMask != 0))
	case gdk.KEY_Escape:
		w.hide()
		return true
	case gdk.KEY_q, gdk.KEY_Q:
		if !ctrl {
			return false
		}
		w.quit()
		return true
	default:
		return false // typing, Ctrl+A, Ctrl+Backspace: the entry has them
	}
	w.render()
	return true
}
