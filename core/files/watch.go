package files

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

// saveEvery is how often a changed index is written back while watching.
const saveEvery = 30 * time.Second

// Watch keeps the index current until ctx ends: inotify for changes, a full
// rescan every rescan (0 never) to catch what inotify missed, and a save to
// cachePath when something changed. inotify watches each directory, so a
// tree larger than fs.inotify.max_user_watches is only partly live.
func (ix *Index) Watch(ctx context.Context, cachePath string, rescan time.Duration, report func(format string, args ...any)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	ix.addWatches(w, ix.dirs(), report)

	save := time.NewTicker(saveEvery)
	defer save.Stop()
	var full <-chan time.Time
	if rescan > 0 {
		t := time.NewTicker(rescan)
		defer t.Stop()
		full = t.C
	}
	flush := func() {
		if ix.isDirty() {
			if err := ix.Save(cachePath); err != nil {
				report("files: could not write cache: %v", err)
			}
		}
	}
	resync := func() {
		if err := ix.Scan(); err != nil {
			report("files: rescan: %v", err)
		}
		ix.addWatches(w, ix.dirs(), report)
		flush()
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			ix.handle(w, ev, report)
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				report("files: inotify queue overflow, rescanning")
				resync()
				continue
			}
			report("files: watch: %v", err)
		case <-save.C:
			flush()
		case <-full:
			resync()
		}
	}
}

func (ix *Index) addWatches(w *fsnotify.Watcher, dirs []string, report func(string, ...any)) {
	failed := 0
	var first error
	for _, d := range dirs {
		if err := w.Add(d); err != nil {
			if first == nil {
				first = err
			}
			failed++
		}
	}
	if failed > 0 {
		report("files: %d of %d folders are not watched live (%v); raise fs.inotify.max_user_watches or narrow files.roots", failed, len(dirs), first)
	}
}

func (ix *Index) handle(w *fsnotify.Watcher, ev fsnotify.Event, report func(string, ...any)) {
	p := ev.Name
	switch {
	case ev.Has(fsnotify.Create):
		st, err := os.Lstat(p)
		if err != nil || ix.excluded(p, st.IsDir()) {
			return
		}
		ix.mu.Lock()
		ix.add(p, st.IsDir())
		ix.mu.Unlock()
		if !st.IsDir() {
			return
		}
		// Each folder is watched before it is listed (WalkDir calls back
		// before reading it), so files created meanwhile are not lost.
		ix.addWatches(w, []string{p}, report)
		var found []entry
		var subdirs []string
		_ = ix.walk(p, func(q string, dir bool) {
			found = append(found, newEntry(q, dir))
			if dir {
				if err := w.Add(q); err != nil {
					subdirs = append(subdirs, q)
				}
			}
		})
		if len(subdirs) > 0 {
			ix.addWatches(w, subdirs, report)
		}
		ix.mu.Lock()
		for _, e := range found {
			ix.add(e.path, e.dir)
		}
		ix.mu.Unlock()
	case ev.Has(fsnotify.Remove), ev.Has(fsnotify.Rename):
		if ev.Has(fsnotify.Rename) {
			_ = w.Remove(p) // the new name arrives as Create
		}
		ix.mu.Lock()
		ix.remove(p)
		ix.mu.Unlock()
	}
}
