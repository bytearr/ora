//go:build !windows

package cli

import (
	"errors"
	"syscall"
)

// readerGone reports a write to a pipe whose reader has exited.
func readerGone(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
