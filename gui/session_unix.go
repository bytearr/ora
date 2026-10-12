//go:build !windows

package gui

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"ora/core/engine"
	"ora/core/index"
	"ora/core/match"
)

// session is the worker side of the window, the same pipeline as on
// Windows: one program search at a time, the file search beside it, older
// generations dropped. post runs a function on the UI thread.
type session struct {
	eng      *engine.Engine
	logf     func(format string, args ...any)
	post     func(func())
	onResult func(Result) // on the UI thread

	latestGen atomic.Uint64
	reqs      chan searchReq
	fileMu    sync.Mutex
	fileNext  *fileReq      // the one pending file search; a newer query replaces it
	fileWake  chan struct{} // wakes fileLoop when fileNext is set
}

type searchReq struct {
	gen    uint64
	query  string
	recent bool
	ix     *index.Index
}

// fileReq is one file search for the window. programs carries the program
// rows of the same query; it is closed without a value if ranking them
// failed.
type fileReq struct {
	gen      uint64
	query    string
	programs chan programRows
}

type programRows struct {
	rows  []Row
	typed *index.Entry
}

func (s *session) start() {
	s.reqs = make(chan searchReq, 1)
	s.fileWake = make(chan struct{}, 1)
	go s.searchLoop()
	go s.fileLoop()
}

func (s *session) recoverWorker(what string) {
	if r := recover(); r != nil {
		s.logf("panic in %s: %v\n%s", what, r, debug.Stack())
	}
}

// loadIndex reads or builds the index off the UI thread.
func (s *session) loadIndex(done func(*index.Index, error)) {
	go func() {
		defer s.recoverWorker("index")
		ix, err := s.eng.Index(false)
		s.post(func() { done(ix, err) })
	}()
}

func (s *session) submit(req searchReq) {
	s.latestGen.Store(req.gen)
	select {
	case <-s.reqs:
	default:
	}
	s.reqs <- req
}

func (s *session) searchLoop() {
	for req := range s.reqs {
		if req.gen != s.latestGen.Load() {
			continue
		}
		s.search(req)
	}
}

func (s *session) deliver(r Result) {
	s.post(func() { s.onResult(r) })
}

// search delivers the answer to req. A text query is answered twice: the
// programs as soon as they are ranked, the full list when the file search
// has replied.
func (s *session) search(req searchReq) {
	defer func() {
		if p := recover(); p != nil {
			s.logf("panic in search: %v\n%s", p, debug.Stack())
			s.deliver(Result{Gen: req.gen, Note: fmt.Sprintf("search failed: %v", p)})
		}
	}()
	if req.recent {
		ids, err := s.eng.RecentIDs()
		if err != nil {
			s.logf("recent: %v", err)
		}
		s.deliver(Result{Gen: req.gen, Rows: DropMissing(RecentRows(ids, req.ix, os.Stat), os.Stat)})
		return
	}
	pr := make(chan programRows, 1)
	defer close(pr)
	s.queueFiles(&fileReq{gen: req.gen, query: req.query, programs: pr})
	programs := DropMissing(ProgramRows(req.ix, s.eng.Search(req.ix, req.query)), os.Stat)
	var typed *index.Entry
	if e, ok := TypedPath(req.query); ok {
		typed = &e
	}
	first := SearchResult(req.gen, programs, typed, nil, nil, nil)
	first.Partial = true
	s.deliver(first)
	pr <- programRows{programs, typed}
}

// queueFiles makes req the pending file search, replacing an older one that
// has not started. It never blocks.
func (s *session) queueFiles(req *fileReq) {
	s.fileMu.Lock()
	s.fileNext = req
	s.fileMu.Unlock()
	select {
	case s.fileWake <- struct{}{}:
	default:
	}
}

// fileLoop runs one file search at a time, always the newest pending one.
func (s *session) fileLoop() {
	for range s.fileWake {
		for {
			s.fileMu.Lock()
			req := s.fileNext
			s.fileNext = nil
			s.fileMu.Unlock()
			if req == nil {
				break
			}
			if req.gen == s.latestGen.Load() {
				s.searchFiles(req)
			}
		}
	}
}

func (s *session) searchFiles(req *fileReq) {
	defer s.recoverWorker("file search")
	ix, ranked, err := func() (ix *index.Index, ranked []match.Result, err error) {
		defer func() {
			if p := recover(); p != nil {
				s.logf("panic in file search: %v\n%s", p, debug.Stack())
				err = fmt.Errorf("file search failed: %v", p)
			}
		}()
		return s.eng.SearchPaths(req.query, true)
	}()
	if req.gen != s.latestGen.Load() {
		return
	}
	p, ok := <-req.programs
	if !ok || req.gen != s.latestGen.Load() {
		return
	}
	s.deliver(SearchResult(req.gen, p.rows, p.typed, ix, ranked, err))
}
