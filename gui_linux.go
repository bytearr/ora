//go:build linux && cgo && !nogui

package main

import "ora/gui"

func init() { openWindow, windowArg = gui.Run, gui.AutostartArg }
