package engine

import (
	"errors"
	"fmt"
	"regexp"

	"ora/core/discover"
	"ora/core/index"
)

// portableSource names the portable-program source in messages.
const portableSource = "everything"

// platformFiles is empty: Everything keeps its own index.
type platformFiles struct{}

func platformSources(regExcl []*regexp.Regexp) []discover.Source {
	return []discover.Source{
		discover.StartMenu{},
		discover.Registry{Exclude: regExcl},
		discover.AppsFolder{},
	}
}

func (e *Engine) refreshFiles() {}

func (e *Engine) portable(evExcl []*regexp.Regexp) ([]index.Entry, error) {
	return discover.Everything{Cfg: e.Cfg.Everything, Exclude: evExcl}.Discover(e.warn)
}

func (e *Engine) findPaths(query string, folders bool) ([]discover.Hit, int, error) {
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
		return nil, 0, ErrNeedsEverything
	}
	if err != nil {
		return nil, 0, fmt.Errorf("everything: %w", err)
	}
	return hits, total, nil
}
