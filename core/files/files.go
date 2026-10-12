// Package files is ora's own file name index for systems without
// Everything: a full scan of the configured roots, a cache file, a live
// name search and, for long-running processes, inotify updates.
package files

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ora/core/config"
)

// Hit is one search result.
type Hit struct {
	Path string
	Dir  bool
}

type entry struct {
	path string
	base string // lower-case file name, what queries match
	dir  bool
}

// Index is safe for concurrent use.
type Index struct {
	roots   []string
	exclude []*regexp.Regexp
	key     string // roots and excludes; a cache with another key is ignored

	mu      sync.RWMutex
	built   time.Time
	entries []entry
	pos     map[string]int
	dirty   bool
}

// New prepares an empty index over roots. exclude is matched against
// directory paths with a trailing slash and against file paths.
func New(roots []string, exclude []*regexp.Regexp) *Index {
	clean := make([]string, 0, len(roots))
	for _, r := range roots {
		if r = strings.TrimSpace(r); r != "" {
			clean = append(clean, filepath.Clean(r))
		}
	}
	h := sha256.New()
	for _, r := range clean {
		fmt.Fprintf(h, "root %s\n", r)
	}
	for _, re := range exclude {
		fmt.Fprintf(h, "exclude %s\n", re.String())
	}
	return &Index{
		roots:   clean,
		exclude: exclude,
		key:     hex.EncodeToString(h.Sum(nil))[:16],
		pos:     map[string]int{},
	}
}

// DefaultRoots is the home folder.
func DefaultRoots() []string {
	if h, err := os.UserHomeDir(); err == nil {
		return []string{h}
	}
	return nil
}

func (ix *Index) Roots() []string { return ix.roots }

func (ix *Index) Built() time.Time {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.built
}

func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.entries)
}

// Stale reports whether the index is older than ttl or was never built.
func (ix *Index) Stale(ttl time.Duration) bool {
	b := ix.Built()
	return b.IsZero() || (ttl > 0 && time.Since(b) > ttl)
}

func (ix *Index) excluded(p string, dir bool) bool {
	if dir {
		p += "/"
	}
	for _, re := range ix.exclude {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

func newEntry(p string, dir bool) entry {
	return entry{path: p, base: strings.ToLower(filepath.Base(p)), dir: dir}
}

// walk lists everything below root that is not excluded, root excluded.
// Unreadable directories are skipped. Symlinks are listed, not followed.
func (ix *Index) walk(root string, fn func(p string, dir bool)) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == root {
			return nil
		}
		if strings.ContainsAny(p, "\n\r") || ix.excluded(p, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		fn(p, d.IsDir())
		return nil
	})
}

// Scan rebuilds the index from disk. The old content stays searchable
// until the new one is complete.
func (ix *Index) Scan() error {
	var es []entry
	var errs []error
	for _, r := range ix.roots {
		err := ix.walk(r, func(p string, dir bool) { es = append(es, newEntry(p, dir)) })
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(ix.roots) > 0 && len(errs) == len(ix.roots) {
		return errors.Join(errs...)
	}
	pos := make(map[string]int, len(es))
	for i, e := range es {
		pos[e.path] = i
	}
	ix.mu.Lock()
	ix.entries, ix.pos, ix.built, ix.dirty = es, pos, time.Now(), true
	ix.mu.Unlock()
	return errors.Join(errs...)
}

const (
	cacheMagic   = "ora-files"
	cacheVersion = 1
)

// Load reads a cache written by Save. ok is false when there is none or it
// belongs to other roots or excludes.
func (ix *Index) Load(path string) (ok bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	if !sc.Scan() {
		return false, sc.Err()
	}
	head := strings.Split(sc.Text(), "\t")
	if len(head) != 4 || head[0] != cacheMagic || head[1] != strconv.Itoa(cacheVersion) || head[3] != ix.key {
		return false, nil
	}
	nanos, err := strconv.ParseInt(head[2], 10, 64)
	if err != nil {
		return false, nil
	}
	var es []entry
	for sc.Scan() {
		line := sc.Text()
		if len(line) < 2 || (line[0] != 'd' && line[0] != 'f') {
			return false, nil
		}
		es = append(es, newEntry(line[1:], line[0] == 'd'))
	}
	if err := sc.Err(); err != nil {
		return false, err
	}
	pos := make(map[string]int, len(es))
	for i, e := range es {
		pos[e.path] = i
	}
	ix.mu.Lock()
	ix.entries, ix.pos, ix.built, ix.dirty = es, pos, time.Unix(0, nanos), false
	ix.mu.Unlock()
	return true, nil
}

// Save writes the index atomically.
func (ix *Index) Save(path string) error {
	ix.mu.RLock()
	var b strings.Builder
	fmt.Fprintf(&b, "%s\t%d\t%d\t%s\n", cacheMagic, cacheVersion, ix.built.UnixNano(), ix.key)
	for _, e := range ix.entries {
		if e.dir {
			b.WriteByte('d')
		} else {
			b.WriteByte('f')
		}
		b.WriteString(e.path)
		b.WriteByte('\n')
	}
	ix.mu.RUnlock()
	if err := config.WriteFileAtomic(path, []byte(b.String())); err != nil {
		return err
	}
	ix.mu.Lock()
	ix.dirty = false
	ix.mu.Unlock()
	return nil
}

// Words splits a query into lower-case words; quotes are dropped.
func Words(query string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if w = strings.ReplaceAll(w, `"`, ""); w != "" {
			out = append(out, w)
		}
	}
	return out
}

// Search returns entries whose name contains every query word, or whose
// full path does when matchPath is set. Folders only with folders. The
// hits are sorted by name and capped at limit; total is the match count.
func (ix *Index) Search(query string, folders, matchPath bool, limit int) ([]Hit, int) {
	words := Words(query)
	if len(words) == 0 {
		return nil, 0
	}
	ix.mu.RLock()
	var found []entry
	for _, e := range ix.entries {
		if e.dir && !folders {
			continue
		}
		hay := e.base
		if matchPath {
			hay = strings.ToLower(e.path)
		}
		all := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				all = false
				break
			}
		}
		if all {
			found = append(found, e)
		}
	}
	ix.mu.RUnlock()
	sort.Slice(found, func(i, j int) bool {
		if found[i].base != found[j].base {
			return found[i].base < found[j].base
		}
		return found[i].path < found[j].path
	})
	total := len(found)
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	hits := make([]Hit, len(found))
	for i, e := range found {
		hits[i] = Hit{Path: e.path, Dir: e.dir}
	}
	return hits, total
}

// WithExt lists the files whose extension is one of exts (no dot, any case).
func (ix *Index) WithExt(exts []string) []string {
	want := map[string]bool{}
	for _, e := range exts {
		if e = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), ".")); e != "" {
			want["."+e] = true
		}
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []string
	for _, e := range ix.entries {
		if !e.dir && want[filepath.Ext(e.base)] {
			out = append(out, e.path)
		}
	}
	sort.Strings(out)
	return out
}

// add inserts or updates one path. Callers hold mu.
func (ix *Index) add(p string, dir bool) {
	if i, ok := ix.pos[p]; ok {
		ix.entries[i].dir = dir
		return
	}
	ix.pos[p] = len(ix.entries)
	ix.entries = append(ix.entries, newEntry(p, dir))
	ix.dirty = true
}

// remove drops p and everything below it. Callers hold mu.
func (ix *Index) remove(p string) {
	if i, ok := ix.pos[p]; ok && !ix.entries[i].dir {
		ix.removeAt(i)
		return
	}
	prefix := p + "/"
	for i := 0; i < len(ix.entries); {
		if e := ix.entries[i].path; e == p || strings.HasPrefix(e, prefix) {
			ix.removeAt(i)
			continue
		}
		i++
	}
}

func (ix *Index) removeAt(i int) {
	last := len(ix.entries) - 1
	delete(ix.pos, ix.entries[i].path)
	if i != last {
		ix.entries[i] = ix.entries[last]
		ix.pos[ix.entries[i].path] = i
	}
	ix.entries = ix.entries[:last]
	ix.dirty = true
}

func (ix *Index) isDirty() bool {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.dirty
}

// dirs lists the roots and every indexed directory.
func (ix *Index) dirs() []string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := append([]string(nil), ix.roots...)
	for _, e := range ix.entries {
		if e.dir {
			out = append(out, e.path)
		}
	}
	return out
}
