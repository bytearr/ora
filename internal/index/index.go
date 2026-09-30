package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"ora/internal/config"
)

type Kind string

const (
	KindShortcut Kind = "shortcut" // Win32 .lnk
	KindExe      Kind = "exe"      // Win32 exe from the uninstall registry
	KindStore    Kind = "store"    // packaged app, launched by AUMID
	KindPortable Kind = "portable" // file found by Everything
	KindFile     Kind = "file"     // any file opened with its default app, never cached
	KindFolder   Kind = "folder"   // a directory opened in Explorer, never cached
)

type Source string

const (
	SourceStartMenu  Source = "startmenu"
	SourceRegistry   Source = "registry"
	SourceAppsFolder Source = "appsfolder"
	SourceEverything Source = "everything"
	SourceSearch     Source = "search" // live `ora file` result or a typed path
)

type Entry struct {
	Name     string `json:"name"`
	Kind     Kind   `json:"kind"`
	Source   Source `json:"source"`
	Target   string `json:"target"`             // .lnk path, exe/script path, or AUMID
	Resolved string `json:"resolved,omitempty"` // .lnk target
	Args     string `json:"args,omitempty"`     // .lnk arguments
	WorkDir  string `json:"workdir,omitempty"`  // .lnk working directory
	AUMID    string `json:"aumid,omitempty"`
	Parent   string `json:"parent,omitempty"` // portable: parent directory name
	Generic  bool   `json:"generic,omitempty"`
}

// ID is stable across rebuilds and used for the recent-launch list.
func (e Entry) ID() string {
	if e.Kind == KindStore {
		return "aumid:" + strings.ToLower(e.AUMID)
	}
	return "path:" + strings.ToLower(e.Target)
}

// Path is the file the entry ultimately runs, used for path dedupe.
func (e Entry) Path() string {
	if e.Kind == KindStore {
		return ""
	}
	if e.Resolved != "" {
		return e.Resolved
	}
	return e.Target
}

const cacheVersion = 1

type Index struct {
	Version int       `json:"version"`
	Built   time.Time `json:"built"`
	Entries []Entry   `json:"entries"`
}

func (ix *Index) Stale(ttl time.Duration) bool {
	return ttl > 0 && time.Since(ix.Built) > ttl
}

func (ix *Index) BySource(s Source) []Entry {
	var out []Entry
	for _, e := range ix.Entries {
		if e.Source == s {
			out = append(out, e)
		}
	}
	return out
}

// Load returns (nil, nil) when there is no usable cache.
func Load() (*Index, error) {
	path, err := config.CacheFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ix Index
	if err := json.Unmarshal(data, &ix); err != nil || ix.Version != cacheVersion {
		return nil, nil
	}
	return &ix, nil
}

func (ix *Index) Save() error {
	path, err := config.CacheFile()
	if err != nil {
		return err
	}
	ix.Version = cacheVersion
	data, err := json.Marshal(ix)
	if err != nil {
		return err
	}
	if err := config.WriteFileAtomic(path, data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
