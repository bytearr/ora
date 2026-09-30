package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"

	"ora/internal/config"
	"ora/internal/discover"
	"ora/internal/index"
	"ora/internal/launch"
	"ora/internal/match"
)

type app struct {
	v       *viper.Viper
	cfg     *config.Config
	verbose bool
	refresh bool
}

func (a *app) warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ora: "+format+"\n", args...)
}

func (a *app) debug(format string, args ...any) {
	if a.verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// loadIndex returns the cached index, rebuilding it per the cache rules in
// PLAN.md: no cache or forceFull -> full build; stale -> sources 1-3 with
// Everything entries carried over.
func (a *app) loadIndex(forceFull bool) (*index.Index, error) {
	if !forceFull {
		ix, err := index.Load()
		if err != nil {
			a.warn("cache unreadable, rebuilding: %v", err)
		}
		if ix != nil && !ix.Stale(a.cfg.IndexTTL) {
			a.debug("index: cache from %s, %d entries", ix.Built.Format(time.RFC3339), len(ix.Entries))
			return ix, nil
		}
		if ix != nil {
			a.debug("index: cache older than %s, rebuilding sources 1-3", a.cfg.IndexTTL)
			return a.build(false, ix.BySource(index.SourceEverything))
		}
	}
	return a.build(true, nil)
}

func (a *app) build(withEverything bool, carry []index.Entry) (*index.Index, error) {
	regExcl, err := discover.ExcludeRegexps(a.cfg.Everything, false)
	if err != nil {
		return nil, err
	}
	evExcl, err := discover.ExcludeRegexps(a.cfg.Everything, true)
	if err != nil {
		return nil, err
	}
	if _, err := discover.CompileRegexps(a.cfg.Everything.IncludeRegex); err != nil {
		return nil, fmt.Errorf("include_regex: %w", err)
	}

	sources := []discover.Source{
		discover.StartMenu{},
		discover.Registry{Exclude: regExcl},
		discover.AppsFolder{},
	}
	var groups [][]index.Entry
	for _, s := range sources {
		groups = append(groups, a.discover(s))
	}

	switch {
	case withEverything && a.cfg.Everything.Enabled:
		start := time.Now()
		es, err := discover.Everything{Cfg: a.cfg.Everything, Exclude: evExcl}.Discover(a.warn)
		switch {
		case errors.Is(err, discover.ErrEverythingNotRunning):
			a.warn("everything: not running, portable programs skipped")
		case err != nil:
			a.warn("everything: %v", err)
		default:
			a.debug("source everything: %d entries in %s", len(es), time.Since(start).Round(time.Millisecond))
			groups = append(groups, es)
		}
	case !withEverything:
		groups = append(groups, carry)
	}

	ix := &index.Index{Built: time.Now(), Entries: index.Merge(groups, a.cfg.Ignore)}
	if err := ix.Save(); err != nil {
		a.warn("could not write cache: %v", err)
	}
	return ix, nil
}

func (a *app) discover(s discover.Source) []index.Entry {
	start := time.Now()
	es, err := s.Discover(a.warn)
	if err != nil {
		a.warn("%s: %v", s.Name(), err)
	}
	a.debug("source %s: %d entries in %s", s.Name(), len(es), time.Since(start).Round(time.Millisecond))
	return es
}

// candidates maps entries to match candidates with aliases and recent flags.
func (a *app) candidates(ix *index.Index, recent []string) []match.Candidate {
	aliasesFor := map[string][]string{}
	for k, official := range a.cfg.Aliases {
		key := match.Compact(match.Normalize(official))
		aliasesFor[key] = append(aliasesFor[key], k)
	}
	isRecent := map[string]bool{}
	for _, id := range recent {
		isRecent[id] = true
	}
	cs := make([]match.Candidate, len(ix.Entries))
	for i, e := range ix.Entries {
		cs[i] = match.Candidate{
			Name:     e.Name,
			Parent:   e.Parent,
			Portable: e.Source == index.SourceEverything,
			Generic:  e.Generic,
			Recent:   isRecent[e.ID()],
			Aliases:  aliasesFor[match.Compact(match.Normalize(e.Name))],
		}
	}
	return cs
}

func (a *app) rank(ix *index.Index, query string) []match.Result {
	recent, err := config.LoadRecent()
	if err != nil {
		a.debug("recent: %v", err)
	}
	return match.Rank(query, a.candidates(ix, recent))
}

func (a *app) launch(ix *index.Index, e index.Entry, extra []string) error {
	if e.Kind == index.KindFile {
		ix = nil // file results are no app index; AutoHotkey is looked up in the cache
	}
	l := launch.Shell{
		FindAutoHotkey: func() string { return findAutoHotkey(ix) },
		Warn:           a.warn,
	}
	a.debug("launch: %s %s -> %s", e.Kind, e.Name, e.Target)
	if err := l.Launch(e, extra); err != nil {
		return &exitErr{code: exitLaunch, msg: fmt.Sprintf("launch %q failed: %v", e.Name, err)}
	}
	if e.Kind == index.KindFile {
		return nil // files are not index entries, a recent bonus would never apply
	}
	recent, _ := config.LoadRecent()
	if err := config.SaveRecent(config.PushRecent(recent, e.ID())); err != nil {
		a.debug("recent: %v", err)
	}
	return nil
}

var autoHotkeyNames = []string{"AutoHotkey64.exe", "AutoHotkeyU64.exe", "AutoHotkey.exe"}

// findAutoHotkey looks in PATH, then in ix; a nil ix means the cached index
// (file launches run without loading it).
func findAutoHotkey(ix *index.Index) string {
	for _, n := range autoHotkeyNames {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	if ix == nil {
		if ix, _ = index.Load(); ix == nil {
			return ""
		}
	}
	for _, n := range autoHotkeyNames {
		for _, e := range ix.Entries {
			if p := e.Path(); p != "" && strings.EqualFold(filepath.Base(p), n) {
				return p
			}
		}
	}
	return ""
}

func describeTarget(e index.Entry) string {
	switch {
	case e.Kind == index.KindStore:
		return e.AUMID
	case e.Resolved != "":
		t := e.Resolved
		if e.Args != "" {
			t += " " + e.Args
		}
		return t
	default:
		return e.Target
	}
}
