package index

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"ora/internal/match"
)

var (
	junkTokens = map[string]bool{
		"readme": true, "manual": true, "license": true,
		"website": true, "help": true, "documentation": true,
	}
	docExts = map[string]bool{
		".url": true, ".chm": true, ".pdf": true, ".txt": true,
		".htm": true, ".html": true, ".rtf": true, ".md": true,
		".ini": true, ".cfg": true, ".log": true,
	}
	genericDropRe = regexp.MustCompile(`^(setup|uninstall|unins\d*|update|updater|crashreporter)$`)
	// Portable files whose name contains one of these words are installers.
	installerTokens = map[string]bool{
		"setup": true, "installer": true, "uninstaller": true, "redist": true, "vcredist": true,
	}
)

// GenericMinCopies is how often a stem must occur among Everything entries
// to count as generic. Two copies are usually a dev and a release build of
// the same tool, not a python.exe-style flood.
const GenericMinCopies = 3

// IsInstallerName reports whether a portable file's display name marks it
// as an installer (VSCodeUserSetup-x64.exe, vc_redist.x64.exe).
func IsInstallerName(name string) bool {
	for _, t := range match.Tokens(match.Normalize(name)) {
		if installerTokens[t] {
			return true
		}
	}
	return false
}

// DropStem reports whether a file stem is an installer/updater helper that
// should never be indexed as its own app.
// Stems without any letter (1.ahk, 2024.bat) are dropped too.
func DropStem(stem string) bool {
	if strings.IndexFunc(stem, unicode.IsLetter) < 0 {
		return true
	}
	return genericDropRe.MatchString(strings.ToLower(stem))
}

// IsJunk applies the junk filter from the plan to one entry.
func IsJunk(e Entry, ignore []string) bool {
	lname := strings.ToLower(e.Name)
	if strings.TrimSpace(lname) == "" || strings.Contains(lname, "uninstall") {
		return true
	}
	words := strings.FieldsFunc(lname, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, t := range words {
		if junkTokens[t] {
			return true
		}
	}
	for _, g := range ignore {
		if ok, _ := path.Match(strings.ToLower(g), lname); ok {
			return true
		}
	}
	for _, p := range []string{e.Target, e.Resolved} {
		lp := strings.ToLower(p)
		if strings.HasPrefix(lp, "http:") || strings.HasPrefix(lp, "https:") || strings.HasPrefix(lp, "mailto:") {
			return true
		}
		// Only for shortcuts/registry: Everything returns exactly the
		// extensions the user configured, including documents.
		if e.Kind != KindStore && e.Source != SourceEverything && docExts[filepath.Ext(lp)] {
			return true
		}
	}
	return false
}

// Merge filters junk and dedupes groups given in source priority order.
// Dedupe stages: AUMID, then resolved path, then normalized name.
func Merge(groups [][]Entry, ignore []string) []Entry {
	var out []Entry
	seenAUMID := map[string]bool{}
	seenPath := map[string]bool{}
	seenName := map[string]bool{}     // names from sources 1-3
	seenPortable := map[string]bool{} // name|parent among Everything entries

	for _, g := range groups {
		for _, e := range g {
			if IsJunk(e, ignore) {
				continue
			}
			aumid := strings.ToLower(e.AUMID)
			if aumid != "" && seenAUMID[aumid] {
				continue
			}
			p := strings.ToLower(filepath.Clean(e.Path()))
			if e.Path() != "" && seenPath[p] {
				continue
			}
			name := match.Compact(match.Normalize(e.Name))
			if seenName[name] {
				continue
			}
			if e.Source == SourceEverything {
				key := name + "|" + match.Compact(match.Normalize(e.Parent))
				if seenPortable[key] {
					continue
				}
				seenPortable[key] = true
			} else {
				seenName[name] = true
			}
			if aumid != "" {
				seenAUMID[aumid] = true
			}
			if e.Path() != "" {
				seenPath[p] = true
			}
			out = append(out, e)
		}
	}
	markGeneric(out)
	return out
}

func markGeneric(es []Entry) {
	count := map[string]int{}
	for _, e := range es {
		if e.Source == SourceEverything {
			count[match.Compact(match.Normalize(e.Name))]++
		}
	}
	for i := range es {
		es[i].Generic = es[i].Source == SourceEverything && count[match.Compact(match.Normalize(es[i].Name))] >= GenericMinCopies
	}
}
