// Package discover collects index entries from the four sources. The pure
// helpers in this file are tested; the COM/registry/IPC parts are not.
package discover

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"ora/internal/config"
	"ora/internal/index"
	"ora/internal/match"
)

// Reporter prints one progress or warning line.
type Reporter func(format string, args ...any)

type Source interface {
	Name() index.Source
	Discover(report Reporter) ([]index.Entry, error)
}

// IsPackagedAUMID reports whether an AUMID belongs to a packaged (Store/MSIX)
// app: PackageFamilyName!AppId.
func IsPackagedAUMID(aumid string) bool {
	i := strings.IndexByte(aumid, '!')
	return i > 0 && i < len(aumid)-1 && !strings.ContainsAny(aumid, `\/`)
}

func underWindowsApps(p string) bool {
	return strings.Contains(strings.ToLower(p), `\windowsapps\`)
}

// ClassifyShortcut turns one .lnk into an entry. A shortcut is a Store app
// only with a packaged AUMID and no usable target outside WindowsApps.
func ClassifyShortcut(lnkPath, target, args, workDir, aumid string) index.Entry {
	name := strings.TrimSuffix(filepath.Base(lnkPath), filepath.Ext(lnkPath))
	if IsPackagedAUMID(aumid) && (target == "" || underWindowsApps(target)) {
		return index.Entry{Name: name, Kind: index.KindStore, Source: index.SourceStartMenu, Target: aumid, AUMID: aumid}
	}
	return index.Entry{
		Name: name, Kind: index.KindShortcut, Source: index.SourceStartMenu,
		Target: lnkPath, Resolved: target, Args: args, WorkDir: workDir, AUMID: aumid,
	}
}

// ParseDisplayIcon strips quotes and a trailing ",N" icon index.
func ParseDisplayIcon(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		if end := strings.Index(s[1:], `"`); end >= 0 {
			return s[1 : end+1]
		}
		return strings.Trim(s, `"`)
	}
	if i := strings.LastIndexByte(s, ','); i > 0 {
		rest := strings.TrimSpace(s[i+1:])
		if rest == "" || strings.TrimLeft(rest, "-0123456789") == "" {
			s = s[:i]
		}
	}
	return strings.TrimSpace(s)
}

// UsableExe reports whether p is an exe that may be launched as an app.
func UsableExe(p string, exclude []*regexp.Regexp) bool {
	if !strings.EqualFold(filepath.Ext(p), ".exe") {
		return false
	}
	if index.DropStem(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))) {
		return false
	}
	for _, re := range exclude {
		if re.MatchString(p) {
			return false
		}
	}
	return true
}

// PickInstallExe returns the only usable exe among the top-level files of an
// install directory, or "" if there is none or more than one.
func PickInstallExe(dir string, files []string, exclude []*regexp.Regexp) string {
	var found string
	for _, f := range files {
		p := filepath.Join(dir, f)
		if !UsableExe(p, exclude) {
			continue
		}
		if found != "" {
			return ""
		}
		found = p
	}
	return found
}

// ExcludeRegexps compiles the configured excludes for client-side checks.
// withProgramFiles adds the Program Files rule used for Everything.
func ExcludeRegexps(ev config.Everything, withProgramFiles bool) ([]*regexp.Regexp, error) {
	var pats []string
	if ev.ExcludeRegex != "" {
		pats = append(pats, ev.ExcludeRegex)
	}
	pats = append(pats, ev.ExtraExcludeRegex...)
	if withProgramFiles {
		pats = append(pats, config.ProgramFilesRegex)
	}
	return CompileRegexps(pats)
}

// CompileRegexps compiles case-insensitive Go regexps.
func CompileRegexps(pats []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, p := range pats {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, fmt.Errorf("regex %q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// EverythingSearch builds the Everything search string (search syntax, not
// regex mode) for the configured extensions, excludes and includes.
func EverythingSearch(ev config.Everything) string {
	var exts []string
	for _, e := range ev.Extensions {
		e = strings.TrimPrefix(strings.TrimSpace(e), ".")
		if e != "" {
			exts = append(exts, e)
		}
	}
	parts := []string{"file:", "ext:" + strings.Join(exts, ";")}
	var excl []string
	if ev.ExcludeRegex != "" {
		excl = append(excl, ev.ExcludeRegex)
	}
	excl = append(excl, ev.ExtraExcludeRegex...)
	excl = append(excl, config.ProgramFilesRegex)
	for _, x := range excl {
		parts = append(parts, `!path:regex:"`+x+`"`)
	}
	if len(ev.IncludeRegex) > 0 {
		parts = append(parts, `path:regex:"(`+strings.Join(ev.IncludeRegex, "|")+`)"`)
	}
	return strings.Join(parts, " ")
}

// FileSearch builds the live search for `ora file`: any file type, each
// query word quoted so Everything operators in it stay literal, plus the
// directory excludes (not Program Files: documents may live there).
func FileSearch(ev config.Everything, query string) string {
	parts := []string{"file:"}
	for _, w := range strings.Fields(query) {
		if w = strings.ReplaceAll(w, `"`, ""); w != "" {
			parts = append(parts, `"`+w+`"`)
		}
	}
	if ev.ExcludeRegex != "" {
		parts = append(parts, `!path:regex:"`+ev.ExcludeRegex+`"`)
	}
	for _, x := range ev.ExtraExcludeRegex {
		parts = append(parts, `!path:regex:"`+x+`"`)
	}
	return strings.Join(parts, " ")
}

// FileEntry builds the entry for a file that is opened with its default
// app. The extension stays in the name so `ora file notes.txt` is exact.
func FileEntry(p string) index.Entry {
	return index.Entry{
		Name:   match.SplitWords(filepath.Base(p)),
		Kind:   index.KindFile,
		Source: index.SourceSearch,
		Target: p,
		Parent: filepath.Base(filepath.Dir(p)),
	}
}

// PortableEntry builds the entry for a file found by Everything. ok is false
// for dropped stems and unsupported extensions.
func PortableEntry(p string, exts []string) (index.Entry, bool) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(p), "."))
	allowed := false
	for _, e := range exts {
		if strings.EqualFold(strings.TrimPrefix(e, "."), ext) {
			allowed = true
		}
	}
	stem := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	name := match.SplitWords(stem)
	if !allowed || stem == "" || index.DropStem(stem) || index.IsInstallerName(name) {
		return index.Entry{}, false
	}
	return index.Entry{
		Name:   name,
		Kind:   index.KindPortable,
		Source: index.SourceEverything,
		Target: p,
		Parent: filepath.Base(filepath.Dir(p)),
	}, true
}
