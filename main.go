package main

import (
	"errors"
	"fmt"
	"os"

	"ora/cli"
)

// openWindow is `ora` without arguments, or hidden with windowArg alone
// (the autostart entry); set where a window exists (gui_windows.go).
// Elsewhere `ora` alone stays the help.
var (
	openWindow func(hidden bool) error
	windowArg  string
)

func main() {
	hidden := len(os.Args) == 2 && windowArg != "" && os.Args[1] == windowArg
	if (len(os.Args) == 1 || hidden) && openWindow != nil {
		if err := openWindow(hidden); err != nil {
			fmt.Fprintln(os.Stderr, "ora:", err)
			os.Exit(cli.ExitOther)
		}
		return
	}
	err := cli.NewRoot().Execute()
	if err == nil {
		return
	}
	var ee *cli.ExitError
	if errors.As(err, &ee) {
		if ee.Error() != "" {
			fmt.Fprintln(os.Stderr, "ora:", ee.Error())
		}
		os.Exit(ee.Code())
	}
	fmt.Fprintln(os.Stderr, "ora:", err)
	os.Exit(cli.ExitOther)
}
