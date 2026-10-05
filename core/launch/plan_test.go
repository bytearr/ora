package launch

import (
	"testing"

	"ora/core/index"
)

func q(s string) string { return "<" + s + ">" }

func TestPlanFor(t *testing.T) {
	lnk := index.Entry{Kind: index.KindShortcut, Target: `C:\SM\Code.lnk`, Resolved: `C:\VS\Code.exe`, Args: "--flag"}
	if p, w := PlanFor(lnk, nil, `E:\proj`, q); p.File != lnk.Target || p.Params != "" || p.Dir != "" || w != "" {
		t.Errorf("lnk without args: %+v %q", p, w)
	}
	if p, _ := PlanFor(lnk, []string{"."}, `E:\proj`, q); p.File != `C:\VS\Code.exe` || p.Params != "--flag <.>" || p.Dir != `E:\proj` {
		t.Errorf("lnk with args: %+v", p)
	}
	noTarget := index.Entry{Kind: index.KindShortcut, Target: `C:\SM\X.lnk`}
	if p, w := PlanFor(noTarget, []string{"a"}, `E:\p`, q); p.File != noTarget.Target || w == "" {
		t.Errorf("lnk without target: %+v %q", p, w)
	}

	exe := index.Entry{Kind: index.KindPortable, Target: `E:\Tools\App\app.exe`}
	if p, _ := PlanFor(exe, nil, `E:\p`, q); p.Dir != `E:\Tools\App` || p.AHK {
		t.Errorf("portable no args: %+v", p)
	}
	if p, _ := PlanFor(exe, []string{"a b", "c"}, `E:\p`, q); p.Dir != `E:\p` || p.Params != "<a b> <c>" {
		t.Errorf("portable args: %+v", p)
	}
	ahk := index.Entry{Kind: index.KindPortable, Target: `E:\s\hot.AHK`}
	if p, _ := PlanFor(ahk, nil, "", q); !p.AHK {
		t.Errorf("ahk not detected: %+v", p)
	}

	store := index.Entry{Kind: index.KindStore, AUMID: "Calc!App", Target: "Calc!App"}
	if p, w := PlanFor(store, []string{"x"}, "", q); !p.Store || p.File != "Calc!App" || w == "" {
		t.Errorf("store: %+v %q", p, w)
	}
}

func TestSelectPath(t *testing.T) {
	exe, ok := SelectPath(index.Entry{Kind: index.KindPortable, Target: `E:\Tools\App\app.exe`})
	if !ok || exe != `E:\Tools\App\app.exe` {
		t.Errorf("portable: %q %v", exe, ok)
	}
	file, ok := SelectPath(index.Entry{Kind: index.KindFile, Target: `D:\Docs\notes.txt`})
	if !ok || file != `D:\Docs\notes.txt` {
		t.Errorf("file: %q %v", file, ok)
	}
	lnk, ok := SelectPath(index.Entry{Kind: index.KindShortcut, Target: `C:\SM\Code.lnk`, Resolved: `C:\VS\Code.exe`})
	if !ok || lnk != `C:\VS\Code.exe` {
		t.Errorf("shortcut must use the resolved exe: %q %v", lnk, ok)
	}
	bare, ok := SelectPath(index.Entry{Kind: index.KindShortcut, Target: `C:\SM\X.lnk`})
	if !ok || bare != `C:\SM\X.lnk` {
		t.Errorf("shortcut without target: %q %v", bare, ok)
	}
	if _, ok := SelectPath(index.Entry{Kind: index.KindStore, AUMID: "Calc!App", Target: "Calc!App"}); ok {
		t.Error("store app has no file")
	}
	if _, ok := SelectPath(index.Entry{Kind: index.KindExe}); ok {
		t.Error("empty target")
	}
}

func TestExplorerSelectArg(t *testing.T) {
	if got := ExplorerSelectArg(`E:\Tools\App\app.exe`); got != `/select,"E:\Tools\App\app.exe"` {
		t.Errorf("plain: %s", got)
	}
	if got := ExplorerSelectArg(`C:\Program Files\App\app.exe`); got != `/select,"C:\Program Files\App\app.exe"` {
		t.Errorf("space: %s", got)
	}
	if got := ExplorerSelectArg(`C:\a"b\app.exe`); got != `/select,"C:\ab\app.exe"` {
		t.Errorf("quote: %s", got)
	}
	if got := ExplorerOpenArg(`E:\Tools`); got != `"E:\Tools"` {
		t.Errorf("folder: %s", got)
	}
	if got := ExplorerOpenArg(`C:\`); got != `"C:\\"` {
		t.Errorf("drive root: %s", got)
	}
}
