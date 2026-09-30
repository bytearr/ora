package launch

import (
	"testing"

	"ora/internal/index"
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
