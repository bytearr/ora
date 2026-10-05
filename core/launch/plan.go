// Package launch starts index entries. PlanFor is pure and tested; the
// ShellExecuteEx / COM part is in launch_windows.go.
package launch

import (
	"path/filepath"
	"strings"

	"ora/core/index"
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

// SelectPath is the file Explorer should highlight. Shortcuts prefer the
// resolved target over the .lnk. Store apps have no file.
func SelectPath(e index.Entry) (string, bool) {
	p := e.Path()
	return p, p != ""
}

// ExplorerSelectArg is the parameter explorer.exe expects. The quotes belong
// to explorer's own command line; a space in the path must stay inside them.
func ExplorerSelectArg(path string) string {
	return "/select," + explorerQuote(path)
}

// ExplorerOpenArg quotes a folder for explorer.exe. A trailing backslash
// would escape the closing quote (`C:\`), so it is doubled.
func ExplorerOpenArg(path string) string {
	return explorerQuote(path)
}

func explorerQuote(path string) string {
	path = strings.ReplaceAll(path, `"`, "")
	if strings.HasSuffix(path, `\`) {
		path += `\`
	}
	return `"` + path + `"`
}

func joinArgs(args []string, quote func(string) string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = quote(a)
	}
	return strings.Join(q, " ")
}
