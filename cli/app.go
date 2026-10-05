package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/viper"

	"ora/core/config"
	"ora/core/engine"
	"ora/core/index"
)

type app struct {
	v       *viper.Viper
	cfg     *config.Config
	eng     *engine.Engine
	verbose bool
	refresh bool
}

func (a *app) warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ora: "+format+"\n", args...)
}

func (a *app) debug(format string, args ...any) {
	if a.verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

func (a *app) launch(ix *index.Index, e index.Entry, extra []string) error {
	if err := a.eng.Launch(ix, e, extra); err != nil {
		return &ExitError{code: exitLaunch, msg: err.Error()}
	}
	return nil
}

func (a *app) reveal(e index.Entry) error {
	err := a.eng.Reveal(e)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, engine.ErrNoPath):
		return &ExitError{code: exitNoMatch, msg: err.Error()}
	default:
		return &ExitError{code: exitLaunch, msg: err.Error()}
	}
}

func describeTarget(e index.Entry) string {
	switch {
	case e.Kind == index.KindStore:
		return e.AUMID
	case e.Resolved != "":
		t := e.Resolved
		if e.Args != "" {
			t += " " + e.Args
		}
		return t
	default:
		return e.Target
	}
}
