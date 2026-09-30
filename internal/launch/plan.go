// Package launch starts index entries. PlanFor is pure and tested; the
// ShellExecuteEx / COM part is in launch_windows.go.
package launch

import (
	"path/filepath"
	"strings"

	"ora/internal/index"
)

type Launcher interface {
	Launch(e index.Entry, extra []string) error
}

type Plan struct {
	Store  bool   // activate AUMID
	AHK    bool   // .ahk script, may need the AutoHotkey fallback
	File   string // what ShellExecuteEx opens
	Params string
	Dir    string
}

// PlanFor decides how an entry is started. warn is non-empty when extra args
// cannot be honored exactly.
func PlanFor(e index.Entry, extra []string, cwd string, quote func(string) string) (p Plan, warn string) {
	params := joinArgs(extra, quote)
	switch e.Kind {
	case index.KindStore:
		if len(extra) > 0 {
			warn = "arguments are ignored for Store apps"
		}
		return Plan{Store: true, File: e.AUMID}, warn

	case index.KindShortcut:
		if len(extra) == 0 {
			return Plan{File: e.Target}, ""
		}
		if e.Resolved != "" {
			return Plan{File: e.Resolved, Params: strings.TrimSpace(e.Args + " " + params), Dir: cwd}, ""
		}
		return Plan{File: e.Target, Params: params, Dir: cwd}, "shortcut has no resolvable target, arguments may be ignored"

	default:
		dir := filepath.Dir(e.Target)
		if len(extra) > 0 {
			dir = cwd
		}
		return Plan{
			File:   e.Target,
			Params: params,
			Dir:    dir,
			AHK:    strings.EqualFold(filepath.Ext(e.Target), ".ahk"),
		}, ""
	}
}

func joinArgs(args []string, quote func(string) string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = quote(a)
	}
	return strings.Join(q, " ")
}
