package discover

import (
	"os"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"

	"ora/internal/index"
)

const uninstallKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

type Registry struct {
	Exclude []*regexp.Regexp
}

func (Registry) Name() index.Source { return index.SourceRegistry }

func (r Registry) Discover(report Reporter) ([]index.Entry, error) {
	type root struct {
		key  registry.Key
		path string
		view uint32
	}
	roots := []root{
		{registry.LOCAL_MACHINE, uninstallKey, registry.WOW64_64KEY},
		{registry.LOCAL_MACHINE, uninstallKey, registry.WOW64_32KEY},
		{registry.CURRENT_USER, uninstallKey, 0},
	}
	var out []index.Entry
	for _, rt := range roots {
		k, err := registry.OpenKey(rt.key, rt.path, registry.ENUMERATE_SUB_KEYS|registry.READ|rt.view)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		for _, n := range names {
			sub, err := registry.OpenKey(k, n, registry.QUERY_VALUE|rt.view)
			if err != nil {
				continue
			}
			if e, ok := r.entry(sub); ok {
				out = append(out, e)
			}
			sub.Close()
		}
		k.Close()
	}
	return out, nil
}

func (r Registry) entry(k registry.Key) (index.Entry, bool) {
	name := regString(k, "DisplayName")
	if name == "" {
		return index.Entry{}, false
	}
	if v, _, err := k.GetIntegerValue("SystemComponent"); err == nil && v == 1 {
		return index.Entry{}, false
	}
	if regString(k, "ParentKeyName") != "" {
		return index.Entry{}, false
	}

	target := ParseDisplayIcon(regString(k, "DisplayIcon"))
	if !(UsableExe(target, r.Exclude) && fileExists(target)) {
		target = ""
		if loc := strings.Trim(regString(k, "InstallLocation"), `" `); loc != "" {
			target = PickInstallExe(loc, topLevelFiles(loc), r.Exclude)
		}
	}
	if target == "" {
		return index.Entry{}, false
	}
	return index.Entry{Name: name, Kind: index.KindExe, Source: index.SourceRegistry, Target: target}, true
}

func regString(k registry.Key, name string) string {
	s, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	if strings.Contains(s, "%") {
		if x, err := registry.ExpandString(s); err == nil {
			s = x
		}
	}
	return strings.TrimSpace(s)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func topLevelFiles(dir string) []string {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, d := range des {
		if !d.IsDir() {
			out = append(out, d.Name())
		}
	}
	return out
}
