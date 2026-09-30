package discover

import (
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"

	"ora/internal/config"
	"ora/internal/index"
)

var ErrEverythingNotRunning = errors.New("Everything is not running")

type Everything struct {
	Cfg     config.Everything
	Exclude []*regexp.Regexp // client-side re-check, including Program Files
}

func (Everything) Name() index.Source { return index.SourceEverything }

func (e Everything) Discover(report Reporter) ([]index.Entry, error) {
	limit := e.Cfg.MaxResults
	paths, total, err := runQuery(e.Cfg, EverythingSearch(e.Cfg), limit, false, false, report)
	if err != nil {
		return nil, err
	}
	if total > len(paths) && len(paths) >= limit {
		report("everything: WARNING cap hit: %d files match, only %d indexed. Tighten exclude_regex or set include_regex.", total, len(paths))
	}

	var out []index.Entry
	dropped := 0
	for _, p := range paths {
		if matchesAny(p, e.Exclude) {
			dropped++
			continue
		}
		if en, ok := PortableEntry(p, e.Cfg.Extensions); ok {
			out = append(out, en)
		}
	}
	if dropped > 0 {
		report("everything: %d results matched an exclude client-side (search syntax not applied by this Everything version?)", dropped)
	}
	return out, nil
}

// runQuery asks the running Everything over IPC, starts it first when it is
// not running and autostart is on, and falls back to es.exe if IPC fails.
func runQuery(ev config.Everything, search string, limit int, matchPath, keepFolders bool, report Reporter) ([]string, int, error) {
	var flags uint32
	if matchPath {
		flags = ipcMatchPath
	}
	paths, total, err := queryIPC(search, uint32(limit), flags, keepFolders)
	if errors.Is(err, ErrEverythingNotRunning) && ev.Autostart {
		if serr := startEverything(report); serr != nil {
			report("everything: %v", serr)
			return nil, 0, err
		}
		paths, total, err = queryIPC(search, uint32(limit), flags, keepFolders)
	}
	if errors.Is(err, ErrEverythingNotRunning) {
		return nil, 0, err
	}
	if err != nil {
		report("everything: IPC failed (%v), trying es.exe", err)
		paths, total, err = queryES(search, limit, matchPath, keepFolders)
		if err != nil {
			return nil, 0, fmt.Errorf("es.exe: %w", err)
		}
	}
	return paths, total, nil
}

// SearchFiles is the live search behind `ora file`: every query word must
// occur in the file name; if nothing matches and there are several words,
// the words may also match folder names. Directory excludes apply.
func SearchFiles(ev config.Everything, query string, limit int, report Reporter) ([]string, int, error) {
	return searchPaths(ev, query, limit, false, report)
}

// SearchOpen is the live search behind `ora open -f`: files and folders.
func SearchOpen(ev config.Everything, query string, limit int, report Reporter) ([]string, int, error) {
	return searchPaths(ev, query, limit, true, report)
}

func searchPaths(ev config.Everything, query string, limit int, keepFolders bool, report Reporter) ([]string, int, error) {
	excl, err := ExcludeRegexps(ev, false)
	if err != nil {
		return nil, 0, err
	}
	search := FileSearch(ev, query)
	if keepFolders {
		search = OpenSearch(ev, query)
	}
	paths, total, err := runQuery(ev, search, limit, false, keepFolders, report)
	if err == nil && len(paths) == 0 && len(strings.Fields(query)) > 1 {
		paths, total, err = runQuery(ev, search, limit, true, keepFolders, report)
	}
	if err != nil {
		return nil, 0, err
	}
	out := paths[:0]
	for _, p := range paths {
		if !matchesAny(p, excl) {
			out = append(out, p)
		}
	}
	return out, total, nil
}

func matchesAny(p string, res []*regexp.Regexp) bool {
	for _, re := range res {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

const (
	ipcMatchPath              = 0x00000004
	ipcRequestFullPathAndName = 0x00000004
	ipcSortNameAscending      = 1
	ipcItemFolder             = 0x00000001
	list2HeaderSize           = 20
	item2Size                 = 8
)

// buildQuery2 encodes EVERYTHING_IPC_QUERY2 followed by the null-terminated
// UTF-16 search string.
func buildQuery2(replyHwnd, replyID, searchFlags, maxResults uint32, search string) []byte {
	s := utf16.Encode([]rune(search))
	buf := make([]byte, 28+2*(len(s)+1))
	le := binary.LittleEndian
	le.PutUint32(buf[0:], replyHwnd)
	le.PutUint32(buf[4:], replyID)
	le.PutUint32(buf[8:], searchFlags)
	le.PutUint32(buf[12:], 0)
	le.PutUint32(buf[16:], maxResults)
	le.PutUint32(buf[20:], ipcRequestFullPathAndName)
	le.PutUint32(buf[24:], ipcSortNameAscending)
	for i, c := range s {
		le.PutUint16(buf[28+2*i:], c)
	}
	return buf
}

// parseList2 decodes an EVERYTHING_IPC_LIST2 reply that carries only
// FULL_PATH_AND_NAME. Returns file paths and the total match count.
func parseList2(b []byte, keepFolders bool) ([]string, int, error) {
	if len(b) < list2HeaderSize {
		return nil, 0, errors.New("everything reply too short")
	}
	le := binary.LittleEndian
	total := int(le.Uint32(b[0:]))
	num := int(le.Uint32(b[4:]))
	if flags := le.Uint32(b[12:]); flags&ipcRequestFullPathAndName == 0 {
		return nil, total, fmt.Errorf("everything reply lacks full paths (flags 0x%x)", flags)
	}
	if list2HeaderSize+num*item2Size > len(b) {
		return nil, total, errors.New("everything reply truncated (items)")
	}
	paths := make([]string, 0, num)
	for i := 0; i < num; i++ {
		it := b[list2HeaderSize+i*item2Size:]
		flags := le.Uint32(it[0:])
		off := int(le.Uint32(it[4:]))
		if off+4 > len(b) {
			return nil, total, errors.New("everything reply truncated (data offset)")
		}
		n := int(le.Uint32(b[off:]))
		if off+4+2*n > len(b) {
			return nil, total, errors.New("everything reply truncated (name)")
		}
		if flags&ipcItemFolder != 0 && !keepFolders {
			continue
		}
		u := make([]uint16, n)
		for j := range u {
			u[j] = le.Uint16(b[off+4+2*j:])
		}
		paths = append(paths, string(utf16.Decode(u)))
	}
	return paths, total, nil
}
