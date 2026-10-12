package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

const (
	appName = "ora"

	DefaultExcludeRegex = `\\(Windows|WinSxS|WindowsApps|WinGet|Package Cache|Installer|node_modules|\.git|Temp|\$Recycle\.Bin|System Volume Information|History|globalStorage|workspaceStorage|CMakeFiles|\.nuget|site-packages|go\\pkg\\mod|resources\\app|target\\(debug|release)\\(build|deps|incremental))\\`
	ProgramFilesRegex   = `^[A-Z]:\\Program Files( \(x86\))?\\`

	// The Linux counterparts: caches, VCS and build trees, and the system
	// prefixes where package managers install programs.
	LinuxExcludeRegex   = `/(\.git|\.hg|\.svn|node_modules|\.cache|\.npm|\.m2|\.gradle|\.cargo|\.rustup|\.local/share/Trash|\.local/share/containers|\.var/app/[^/]+/cache|__pycache__|\.venv|venv|site-packages|go/pkg/mod|target/(debug|release)|CMakeFiles|\.vscode-server|\.copilot|snap/[^/]+/common/\.cache)/`
	LinuxInstalledRegex = `^/(usr|opt|snap|var/lib/flatpak|nix/store)/`
)

// Files configures ora's own file index, the Everything replacement where
// there is no Everything (Linux). Windows ignores it.
type Files struct {
	// Roots are the trees that are indexed; empty means the home folder.
	Roots []string `mapstructure:"roots"`
	// Rescan is how old the index may get before a full rescan.
	Rescan time.Duration `mapstructure:"rescan"`
	// Watch keeps long-running processes (ora mcp) current with inotify.
	Watch bool `mapstructure:"watch"`
}

type Everything struct {
	Enabled           bool     `mapstructure:"enabled"`
	Autostart         bool     `mapstructure:"autostart"`
	Extensions        []string `mapstructure:"extensions"`
	ExcludeRegex      string   `mapstructure:"exclude_regex"`
	ExtraExcludeRegex []string `mapstructure:"extra_exclude_regex"`
	IncludeRegex      []string `mapstructure:"include_regex"`
	MaxResults        int      `mapstructure:"max_results"`
}

// Settings are the window options. The window reads them once at start.
type Settings struct {
	Hotkey  string `mapstructure:"hotkey"`
	Monitor string `mapstructure:"monitor"`
	Theme   string `mapstructure:"theme"`
	// Autostart keeps a Run entry for ora in the registry; false removes it.
	Autostart bool `mapstructure:"autostart"`
}

type Config struct {
	MinScore     float64           `mapstructure:"min_score"`
	AmbiguityGap float64           `mapstructure:"ambiguity_gap"`
	IndexTTL     time.Duration     `mapstructure:"index_ttl"`
	Aliases      map[string]string `mapstructure:"aliases"`
	Ignore       []string          `mapstructure:"ignore"`
	Everything   Everything        `mapstructure:"everything"`
	Files        Files             `mapstructure:"files"`
	Settings     Settings          `mapstructure:"settings"`
}

func SetDefaults(v *viper.Viper) {
	v.SetDefault("settings.hotkey", DefaultHotkey)
	v.SetDefault("settings.monitor", MonitorPrimary)
	v.SetDefault("settings.theme", ThemeDark)
	v.SetDefault("settings.autostart", false)
	v.SetDefault("min_score", 0.86)
	v.SetDefault("ambiguity_gap", 0.12)
	v.SetDefault("index_ttl", "24h")
	v.SetDefault("aliases", map[string]string{})
	v.SetDefault("ignore", []string{"*uninstall*"})
	v.SetDefault("everything.enabled", true)
	v.SetDefault("everything.autostart", true)
	v.SetDefault("everything.extensions", platformExtensions)
	v.SetDefault("everything.exclude_regex", platformExcludeRegex)
	v.SetDefault("everything.extra_exclude_regex", []string{})
	v.SetDefault("everything.include_regex", []string{})
	v.SetDefault("everything.max_results", 20000)
	v.SetDefault("files.roots", []string{})
	v.SetDefault("files.rescan", "1h")
	v.SetDefault("files.watch", true)
}

// Load reads config.yaml into v (missing file means defaults) and decodes it.
// Flags must be bound to v before calling Load.
func Load(v *viper.Viper) (*Config, error) {
	SetDefaults(v)
	path, err := ConfigFile()
	if err != nil {
		return nil, err
	}
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if c.Aliases == nil {
		c.Aliases = map[string]string{}
	}
	if c.Everything.MaxResults <= 0 {
		c.Everything.MaxResults = 20000
	}
	return &c, nil
}

func ConfigFile() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yaml"), nil
}

func CacheFile() (string, error) {
	d, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "index.json"), nil
}

// FilesCacheFile holds ora's own file index (Linux).
func FilesCacheFile() (string, error) {
	d, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "files.idx"), nil
}

func RecentFile() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "recent.json"), nil
}

// WriteFileAtomic writes via a temp file in the same directory and renames it.
func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
