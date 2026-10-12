package cli

import (
	"fmt"

	"ora/core/index"
)

const (
	fileSearchName  = "Everything"
	fileManagerName = "Explorer"
	appKind         = "a Windows app"
)

func sourceCounts(c map[index.Source]int) string {
	return fmt.Sprintf("startmenu %d, registry %d, appsfolder %d, everything %d",
		c[index.SourceStartMenu], c[index.SourceRegistry], c[index.SourceAppsFolder], c[index.SourceEverything])
}
