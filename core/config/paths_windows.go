package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	platformExcludeRegex = DefaultExcludeRegex
	// InstalledRegex marks installed programs; the portable scan skips them.
	InstalledRegex = ProgramFilesRegex
)

var platformExtensions = []string{"exe", "ahk", "cmd", "bat"}

func envDir(env string) (string, error) {
	base := os.Getenv(env)
	if base == "" {
		return "", fmt.Errorf("%%%s%% is not set", env)
	}
	return filepath.Join(base, appName), nil
}

// Dir is %APPDATA%\ora (config and recent list).
func Dir() (string, error) { return envDir("APPDATA") }

// CacheDir is %LOCALAPPDATA%\ora (index cache).
func CacheDir() (string, error) { return envDir("LOCALAPPDATA") }
