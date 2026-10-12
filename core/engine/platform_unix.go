//go:build !windows

package engine

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"ora/core/config"
	"ora/core/discover"
	"ora/core/files"
	"ora/core/index"
)

// portableSource names the portable-program source in messages.
const portableSource = "portable"

// platformFiles is ora's own file index, opened on first use.
type platformFiles struct {
	once    sync.Once
	ix      *files.Index
	err     error
	path    string
	scanned bool // built in this process, no rescan needed for `ora index`
	watched bool
}

func platformSources([]*regexp.Regexp) []discover.Source {
	return []discover.Source{discover.Desktop{}}
}

// fileRoots are the configured roots with ~ expanded, or the home folder.
func (e *Engine) fileRoots() []string {
	home, _ := os.UserHomeDir()
	var out []string
	for _, r := range e.Cfg.Files.Roots {
		r = strings.TrimSpace(r)
		if r == "~" {
			r = home
		} else if strings.HasPrefix(r, "~/") {
			r = filepath.Join(home, r[2:])
		}
		if r != "" {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return files.DefaultRoots()
	}
	return out
}

func (e *Engine) fileIndex() (*files.Index, error) {
	f := &e.files
	f.once.Do(func() { f.ix, f.err = e.openFiles() })
	return f.ix, f.err
}

// openFiles loads the cached file index. A missing one is built now; a
// stale one is rebuilt now, or in the background when LiveFiles is set.
func (e *Engine) openFiles() (*files.Index, error) {
	f := &e.files
	excl, err := discover.ExcludeRegexps(e.Cfg.Everything, false)
	if err != nil {
		return nil, err
	}
	ix := files.New(e.fileRoots(), excl)
	if f.path, err = config.FilesCacheFile(); err != nil {
		return nil, err
	}
	ok, err := ix.Load(f.path)
	if err != nil {
		e.warn("files: cache unreadable, rebuilding: %v", err)
	}
	live := e.LiveFiles && e.Cfg.Files.Watch
	switch {
	case ok && !ix.Stale(e.Cfg.Files.Rescan):
		e.debug("files: cache from %s, %d entries", ix.Built().Format(time.RFC3339), ix.Len())
	case ok && live:
		e.debug("files: cache older than %s, rescanning in the background", e.Cfg.Files.Rescan)
		f.watched = true
		go func() {
			e.scanFiles(ix)
			e.watchFiles(ix)
		}()
		return ix, nil
	default:
		if !ok {
			e.warn("files: building the file index of %s (first run)", strings.Join(ix.Roots(), ", "))
		}
		if err := e.scanFiles(ix); err != nil && ix.Len() == 0 {
			return nil, err
		}
		f.scanned = true
	}
	if live {
		f.watched = true
		go e.watchFiles(ix)
	}
	return ix, nil
}

func (e *Engine) scanFiles(ix *files.Index) error {
	start := time.Now()
	err := ix.Scan()
	if err != nil {
		e.warn("files: %v", err)
	}
	e.debug("files: scanned %d entries in %s", ix.Len(), time.Since(start).Round(time.Millisecond))
	if serr := ix.Save(e.files.path); serr != nil {
		e.warn("files: could not write cache: %v", serr)
	}
	return err
}

func (e *Engine) watchFiles(ix *files.Index) {
	if err := ix.Watch(context.Background(), e.files.path, e.Cfg.Files.Rescan, e.warn); err != nil {
		e.warn("files: live updates off: %v", err)
	}
}

// refreshFiles rescans for `ora index` unless this process just did.
func (e *Engine) refreshFiles() {
	ix, err := e.fileIndex()
	if err != nil || e.files.scanned || e.files.watched {
		return
	}
	e.scanFiles(ix)
	e.files.scanned = true
}

// portable lists the files with the configured extensions (AppImage by
// default) outside the system prefixes.
func (e *Engine) portable(evExcl []*regexp.Regexp) ([]index.Entry, error) {
	ix, err := e.fileIndex()
	if err != nil {
		return nil, err
	}
	var out []index.Entry
	for _, p := range ix.WithExt(e.Cfg.Everything.Extensions) {
		if matchesAny(p, evExcl) {
			continue
		}
		if en, ok := discover.PortableEntry(p, e.Cfg.Everything.Extensions); ok {
			out = append(out, en)
		}
		if len(out) >= e.Cfg.Everything.MaxResults {
			e.warn("portable: cap of %d entries hit; narrow files.roots or exclude_regex", len(out))
			break
		}
	}
	return out, nil
}

func matchesAny(p string, res []*regexp.Regexp) bool {
	for _, re := range res {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

// findPaths searches file names; with several words and no hit, the words
// may match folder names in the path too (as with Everything).
func (e *Engine) findPaths(query string, folders bool) ([]discover.Hit, int, error) {
	ix, err := e.fileIndex()
	if err != nil {
		return nil, 0, err
	}
	hits, total := ix.Search(query, folders, false, fileSearchLimit)
	if len(hits) == 0 && len(files.Words(query)) > 1 {
		hits, total = ix.Search(query, folders, true, fileSearchLimit)
	}
	out := make([]discover.Hit, len(hits))
	for i, h := range hits {
		out[i] = discover.Hit{Path: h.Path, Dir: h.Dir}
	}
	return out, total, nil
}
