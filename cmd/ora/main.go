package main

import (
	"errors"
	"fmt"
	"os"
)

const (
	exitLaunched  = 0
	exitNoMatch   = 1
	exitAmbiguous = 2
	exitLaunch    = 3
	exitError     = 4
)

type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string { return e.msg }

func main() {
	err := newRoot().Execute()
	if err == nil {
		return
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		if ee.msg != "" {
			fmt.Fprintln(os.Stderr, "ora:", ee.msg)
		}
		os.Exit(ee.code)
	}
	fmt.Fprintln(os.Stderr, "ora:", err)
	os.Exit(exitError)
}
