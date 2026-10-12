//go:build !windows

package discover

import "errors"

// Everything exists only on Windows; elsewhere ora uses core/files.

func queryIPC(string, uint32, uint32, bool) ([]Hit, int, error) {
	return nil, 0, ErrEverythingNotRunning
}

func startEverything(Reporter) error {
	return errors.New("Everything is Windows-only")
}

func queryES(string, int, bool, bool) ([]Hit, int, error) {
	return nil, 0, ErrEverythingNotRunning
}
