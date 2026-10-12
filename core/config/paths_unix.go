//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

const (
	platformExcludeRegex = LinuxExcludeRegex
	// InstalledRegex marks installed programs; the portable scan skips them.
	InstalledRegex = LinuxInstalledRegex
)

var platformExtensions = []string{"AppImage"}

// Dir is $XDG_CONFIG_HOME/ora, usually ~/.config/ora (config and recent list).
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName), nil
}

// CacheDir is $XDG_CACHE_HOME/ora, usually ~/.cache/ora (index caches).
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName), nil
}
