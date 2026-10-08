// Package engine is the shared backend of the terminal and the window:
// loading and building the index, ranking, launching and revealing. It
// decides nothing; the caller picks the entry.
package engine

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"

	"ora/core/config"
	"ora/core/discover"
	"ora/core/index"
	"ora/core/launch"
	"ora/core/match"
)

type WarnFunc func(format string, args ...any)

type Engine struct {
	Cfg  *config.Config
	Warn WarnFunc
	// Debug prints sources, timings and launch targets; nil is silent.
	Debug WarnFunc

	mu      sync.Mutex
	candsIx *index.Index // index the cached candidates belong to
	cands   []match.Candidate
	ids     []string
}

// ErrNeedsEverything is returned by SearchPaths when Everything is not running.
var ErrNeedsEverything = errors.New("file search needs Everything (https://www.voidtools.com)")

// ErrNoPath is returned by Reveal for entries without a file, e.g. Store apps.
var ErrNoPath = errors.New("no file path")

// fileSearchLimit caps one live file query; Everything sorts by name,
// so a cap hit means the query is too broad.
const fileSearchLimit = 2000

// Open reads config.yaml with defaults.
func Open(warn WarnFunc) (*Engine, error) {
	cfg, err := config.Load(viper.New())
	if err != nil {
		return nil, err
	}
	return &Engine{Cfg: cfg, Warn: warn}, nil
}

func (e *Engine) warn(format string, args ...any) {
	if e.Warn != nil {
		e.Warn(format, args...)
	}
}

func (e *Engine) debug(format string, args ...any) {
	if e.Debug != nil {
		e.Debug(format, args...)
	}
}

// Index returns the cached index. No cache or force scans every source.
// A stale cache rebuilds Start Menu, registry and Store apps and keeps the
// Everything entries.
func (e *Engine) Index(force bool) (*index.Index, error) {
	if !force {
		ix, err := index.Load()
		if err != nil {
			e.warn("cache unreadable, rebuilding: %v", err)
		}
		if ix != nil && !ix.Stale(e.Cfg.IndexTTL) {
			e.debug("index: cache from %s, %d entries", ix.Built.Format(time.RFC3339), len(ix.Entries))
			return ix, nil
		}
		if ix != nil {
			e.debug("index: cache older than %s, rebuilding sources 1-3", e.Cfg.IndexTTL)
			return e.build(false, ix.BySource(index.SourceEverything))
		}
	}
	return e.build(true, nil)
}

func (e *Engine) build(withEverything bool, carry []index.Entry) (*index.Index, error) {
	regExcl, err := discover.ExcludeRegexps(e.Cfg.Everything, false)
	if err != nil {
		return nil, err
	}
	evExcl, err := discover.ExcludeRegexps(e.Cfg.Everything, true)
	if err != nil {
		return nil, err
	}
	if _, err := discover.CompileRegexps(e.Cfg.Everything.IncludeRegex); err != nil {
		return nil, fmt.Errorf("include_regex: %w", err)
	}

	sources := []discover.Source{
		discover.StartMenu{},
		discover.Registry{Exclude: regExcl},
		discover.AppsFolder{},
	}
	var groups [][]index.Entry
	for _, s := range sources {
		groups = append(groups, e.discover(s))
	}

	switch {
	case withEverything && e.Cfg.Everything.Enabled:
		start := time.Now()
		es, err := discover.Everything{Cfg: e.Cfg.Everything, Exclude: evExcl}.Discover(e.warn)
		switch {
		case errors.Is(err, discover.ErrEverythingNotRunning):
			e.warn("everything: not running, portable programs skipped")
		case err != nil:
			e.warn("everything: %v", err)
		default:
			e.debug("source everything: %d entries in %s", len(es), time.Since(start).Round(time.Millisecond))
			groups = append(groups, es)
		}
	case !withEverything:
		groups = append(groups, carry)
	}

	ix := &index.Index{Built: time.Now(), Entries: index.Merge(groups, e.Cfg.Ignore)}
	if err := ix.Save(); err != nil {
		e.warn("could not write cache: %v", err)
	}
	return ix, nil
}

func (e *Engine) discover(s discover.Source) []index.Entry {
	start := time.Now()
	es, err := s.Discover(e.warn)
	if err != nil {
		e.warn("%s: %v", s.Name(), err)
	}
	e.debug("source %s: %d entries in %s", s.Name(), len(es), time.Since(start).Round(time.Millisecond))
	return es
}

// candidates maps entries to match candidates with aliases and recent flags.
// The prepared list is built once per index; aliases are read then, so the
// index must not change in place. Callers hold e.mu.
func (e *Engine) candidates(ix *index.Index, recent []string) []match.Candidate {
	if e.candsIx != ix {
		aliasesFor := map[string][]string{}
		for k, official := range e.Cfg.Aliases {
			key := match.Compact(match.Normalize(official))
			aliasesFor[key] = append(aliasesFor[key], k)
		}
		e.cands = make([]match.Candidate, len(ix.Entries))
		e.ids = make([]string, len(ix.Entries))
		for i, en := range ix.Entries {
			e.cands[i] = match.Candidate{
				Name:     en.Name,
				Parent:   en.Parent,
				Portable: en.Source == index.SourceEverything,
				Generic:  en.Generic,
				Aliases:  aliasesFor[match.Compact(match.Normalize(en.Name))],
			}
			e.ids[i] = en.ID()
		}
		match.Prepare(e.cands)
		e.candsIx = ix
	}
	isRecent := map[string]bool{}
	for _, id := range recent {
		isRecent[id] = true
	}
	for i := range e.cands {
		e.cands[i].Recent = isRecent[e.ids[i]]
	}
	return e.cands
}

// Search ranks every index entry against query with aliases and the recent
// bonus. The full list, sorted by score.
func (e *Engine) Search(ix *index.Index, query string) []match.Result {
	recent, err := config.LoadRecent()
	if err != nil {
		e.debug("recent: %v", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return match.Rank(query, e.candidates(ix, recent))
}

// SearchPaths asks Everything for files (and folders when folders is set)
// and ranks them. The returned index holds only these paths. No match is an
// empty result, not an error.
func (e *Engine) SearchPaths(query string, folders bool) (*index.Index, []match.Result, error) {
	start := time.Now()
	var hits []discover.Hit
	var total int
	var err error
	if folders {
		hits, total, err = discover.SearchOpen(e.Cfg.Everything, query, fileSearchLimit, e.warn)
	} else {
		var paths []string
		paths, total, err = discover.SearchFiles(e.Cfg.Everything, query, fileSearchLimit, e.warn)
		for _, p := range paths {
			hits = append(hits, discover.Hit{Path: p})
		}
	}
	if errors.Is(err, discover.ErrEverythingNotRunning) {
		return nil, nil, ErrNeedsEverything
	}
	if err != nil {
		return nil, nil, fmt.Errorf("everything: %w", err)
	}
	e.debug("file search: %d of %d results in %s", len(hits), total, time.Since(start).Round(time.Millisecond))
	if total > len(hits) && len(hits) >= fileSearchLimit {
		e.warn("%d files match, only the first %d (by name) are ranked; add a word to narrow it", total, len(hits))
	}
	ix := &index.Index{Entries: make([]index.Entry, len(hits))}
	cs := make([]match.Candidate, len(hits))
	for i, h := range hits {
		if h.Dir {
			ix.Entries[i] = discover.DirEntry(h.Path)
		} else {
			ix.Entries[i] = discover.FileEntry(h.Path)
		}
		cs[i] = match.Candidate{Name: ix.Entries[i].Name, Parent: ix.Entries[i].Parent}
	}
	return ix, match.Rank(query, cs), nil
}

// Launch starts entry. Programs go into the recent list, files do not.
func (e *Engine) Launch(ix *index.Index, entry index.Entry, extra []string) error {
	if entry.Kind == index.KindFile {
		ix = nil // file results are no app index; AutoHotkey is looked up in the cache
	}
	l := launch.Shell{
		FindAutoHotkey: func() string { return findAutoHotkey(ix) },
		Warn:           e.warn,
	}
	e.debug("launch: %s %s -> %s", entry.Kind, entry.Name, entry.Target)
	if err := l.Launch(entry, extra); err != nil {
		return fmt.Errorf("launch %q failed: %w", entry.Name, err)
	}
	if entry.Kind == index.KindFile {
		return nil // files are not index entries, a recent bonus would never apply
	}
	e.push(entry)
	return nil
}

// Remember puts entry in front of the recent list.
func (e *Engine) Remember(entry index.Entry) {
	e.push(entry)
}

func (e *Engine) push(entry index.Entry) {
	recent, _ := config.LoadRecent()
	if err := config.SaveRecent(config.PushRecent(recent, entry.ID())); err != nil {
		e.debug("recent: %v", err)
	}
}

// Reveal shows entry in Explorer: a folder is opened, anything else is
// highlighted in its folder.
func (e *Engine) Reveal(entry index.Entry) error {
	p, ok := launch.SelectPath(entry)
	if !ok {
		return fmt.Errorf("%s has %w", entry.Name, ErrNoPath)
	}
	e.debug("open: %s %s -> %s", entry.Kind, entry.Name, p)
	var err error
	if entry.Kind == index.KindFolder {
		err = launch.OpenDir(p)
	} else {
		err = launch.Reveal(p)
	}
	if err != nil {
		return fmt.Errorf("open %q failed: %w", entry.Name, err)
	}
	return nil
}

// RecentIDs is the recent list, newest first.
func (e *Engine) RecentIDs() ([]string, error) {
	return config.LoadRecent()
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
		for _, en := range ix.Entries {
			if p := en.Path(); p != "" && strings.EqualFold(filepath.Base(p), n) {
				return p
			}
		}
	}
	return ""
}
