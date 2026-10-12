//go:build !windows

package cli

import (
	"fmt"

	"ora/core/index"
)

const (
	fileSearchName  = "ora's file index"
	fileManagerName = "the file manager"
	appKind         = "an app"
)

func sourceCounts(c map[index.Source]int) string {
	return fmt.Sprintf("desktop %d, portable %d", c[index.SourceDesktop], c[index.SourceEverything])
}
