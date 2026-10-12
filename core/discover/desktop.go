package discover

import (
	"bufio"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ora/core/index"
)

// DesktopFile is the [Desktop Entry] group of a .desktop file, already
// localized.
type DesktopFile struct {
	Type       string
	Name       string
	Exec       string
	TryExec    string
	Path       string
	Icon       string
	Terminal   bool
	NoDisplay  bool
	Hidden     bool
	OnlyShowIn []string
	NotShowIn  []string
}

// ParseDesktop reads the [Desktop Entry] group. locales are tried in order
// for localized keys (see Locales); the unlocalized value is the fallback.
func ParseDesktop(data string, locales []string) DesktopFile {
	vals := map[string]string{}
	in := false
	sc := bufio.NewScanner(strings.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			in = line == "[Desktop Entry]"
			continue
		}
		if !in {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if _, dup := vals[k]; !dup {
			vals[k] = strings.TrimSpace(v)
		}
	}
	loc := func(key string) string {
		for _, l := range locales {
			if v, ok := vals[key+"["+l+"]"]; ok {
				return UnescapeDesktop(v)
			}
		}
		return UnescapeDesktop(vals[key])
	}
	return DesktopFile{
		Type:       vals["Type"],
		Name:       loc("Name"),
		Exec:       vals["Exec"], // unescaped by the launcher, quoting is part of it
		TryExec:    UnescapeDesktop(vals["TryExec"]),
		Path:       UnescapeDesktop(vals["Path"]),
		Icon:       loc("Icon"),
		Terminal:   vals["Terminal"] == "true",
		NoDisplay:  vals["NoDisplay"] == "true",
		Hidden:     vals["Hidden"] == "true",
		OnlyShowIn: desktopList(vals["OnlyShowIn"]),
		NotShowIn:  desktopList(vals["NotShowIn"]),
	}
}

// UnescapeDesktop applies the general escapes of desktop file values.
func UnescapeDesktop(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func desktopList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ";") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Locales turns a POSIX locale (de_DE.UTF-8@euro) into the lookup order of
// the desktop entry spec: de_DE@euro, de_DE, de@euro, de.
func Locales(posix string) []string {
	if posix == "" || posix == "C" || posix == "POSIX" || strings.HasPrefix(posix, "C.") {
		return nil
	}
	lang, mod, _ := strings.Cut(posix, "@")
	lang, _, _ = strings.Cut(lang, ".")
	l, country, hasCountry := strings.Cut(lang, "_")
	var out []string
	if hasCountry && mod != "" {
		out = append(out, l+"_"+country+"@"+mod)
	}
	if hasCountry {
		out = append(out, l+"_"+country)
	}
	if mod != "" {
		out = append(out, l+"@"+mod)
	}
	return append(out, l)
}

// Visible applies Hidden, NoDisplay, Type and OnlyShowIn/NotShowIn for the
// current desktops (XDG_CURRENT_DESKTOP, colon separated).
func (d DesktopFile) Visible(desktops []string) bool {
	if d.Hidden || d.NoDisplay || d.Type != "Application" || d.Name == "" || d.Exec == "" {
		return false
	}
	has := func(list []string) bool {
		for _, x := range list {
			for _, y := range desktops {
				if strings.EqualFold(x, y) {
					return true
				}
			}
		}
		return false
	}
	if len(d.OnlyShowIn) > 0 && !has(d.OnlyShowIn) {
		return false
	}
	return !has(d.NotShowIn)
}

// DesktopID is the desktop file id: the path below applications/ with
// slashes turned into dashes (kde/konsole.desktop -> kde-konsole.desktop).
func DesktopID(appsDir, file string) string {
	rel, err := filepath.Rel(appsDir, file)
	if err != nil {
		return filepath.Base(file)
	}
	return strings.ReplaceAll(filepath.ToSlash(rel), "/", "-")
}

// DesktopEntry is the index entry for a visible desktop file. The program
// is not used as its path: several entries may run one binary
// (libreoffice --writer, --calc) and must not be merged.
func DesktopEntry(path string, d DesktopFile) index.Entry {
	return index.Entry{
		Name:    d.Name,
		Kind:    index.KindDesktop,
		Source:  index.SourceDesktop,
		Target:  path,
		WorkDir: d.Path,
	}
}

// Desktop collects the XDG application entries: $XDG_DATA_HOME/applications
// first, then each $XDG_DATA_DIRS/applications. The first file with a given
// id wins, so a user copy with Hidden=true removes a system entry.
type Desktop struct{}

func (Desktop) Name() index.Source { return index.SourceDesktop }

func (Desktop) Discover(report Reporter) ([]index.Entry, error) {
	locales := Locales(firstEnv("LC_ALL", "LC_MESSAGES", "LANG"))
	desktops := desktopList(strings.ReplaceAll(os.Getenv("XDG_CURRENT_DESKTOP"), ":", ";"))
	seen := map[string]bool{}
	var out []index.Entry
	for _, dir := range ApplicationDirs() {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() && p != dir {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() || !strings.HasSuffix(p, ".desktop") {
				return nil
			}
			id := DesktopID(dir, p)
			if seen[id] {
				return nil
			}
			seen[id] = true
			data, err := os.ReadFile(p)
			if err != nil {
				report("desktop: %v", err)
				return nil
			}
			df := ParseDesktop(string(data), locales)
			if !df.Visible(desktops) || !tryExecOK(df.TryExec) {
				return nil
			}
			out = append(out, DesktopEntry(p, df))
			return nil
		})
	}
	return out, nil
}

func tryExecOK(p string) bool {
	if p == "" {
		return true
	}
	_, err := exec.LookPath(p)
	return err == nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// ApplicationDirs lists the applications folders in priority order.
func ApplicationDirs() []string {
	home := os.Getenv("XDG_DATA_HOME")
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(h, ".local", "share")
		}
	}
	dirs := os.Getenv("XDG_DATA_DIRS")
	if dirs == "" {
		dirs = "/usr/local/share:/usr/share"
	}
	var out []string
	seen := map[string]bool{}
	for _, d := range append([]string{home}, strings.Split(dirs, ":")...) {
		if d == "" {
			continue
		}
		a := filepath.Join(d, "applications")
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}
