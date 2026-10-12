package cli

import (
	"errors"

	"golang.org/x/sys/windows"
)

// readerGone reports a write to a pipe whose reader has exited.
func readerGone(err error) bool {
	return errors.Is(err, windows.ERROR_NO_DATA) || errors.Is(err, windows.ERROR_BROKEN_PIPE)
}
