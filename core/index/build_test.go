package index

import "testing"

func names(es []Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

func TestJunk(t *testing.T) {
	junk := []Entry{
		{Name: "Uninstall Foo", Target: `C:\foo\unins000.exe`},
		{Name: "Foo Readme", Target: `C:\foo\readme.lnk`},
		{Name: "Foo Help", Target: `C:\foo\help.chm`},
		{Name: "Foo Website", Target: `https://foo.example`},
		{Name: "Mail Foo", Resolved: `mailto:foo@example.com`, Target: `C:\x.lnk`},
		{Name: "Foo Docs", Target: `C:\x.lnk`, Resolved: `C:\foo\docs.pdf`},
		{Name: "Ignored Thing", Target: `C:\a.exe`},
	}
	for _, e := range junk {
		if !IsJunk(e, []string{"ignored*"}) {
			t.Errorf("not junk: %+v", e)
		}
	}
	keep := []Entry{
		{Name: "HelpNDoc", Target: `C:\h\helpndoc.exe`},
		{Name: "Discord", Target: `C:\d.lnk`, Resolved: `C:\d\Update.exe`},
		{Name: "Calculator", Kind: KindStore, Target: "Microsoft.WindowsCalculator_8wekyb3d8bbwe!App", AUMID: "Microsoft.WindowsCalculator_8wekyb3d8bbwe!App"},
		// Everything only returns extensions the user configured.
		{Name: "Notes", Kind: KindPortable, Source: SourceEverything, Target: `D:\docs\notes.txt`},
	}
	for _, e := range keep {
		if IsJunk(e, []string{"*uninstall*"}) {
			t.Errorf("junk: %+v", e)
		}
	}
}

func TestDropStem(t *testing.T) {
	for _, s := range []string{"setup", "Uninstall", "unins000", "unins001", "update", "Updater", "crashreporter", "1", "2024", ""} {
		if !DropStem(s) {
			t.Errorf("not dropped: %s", s)
		}
	}
	for _, s := range []string{"setuptools", "discord", "PixelPaintStudio"} {
		if DropStem(s) {
			t.Errorf("dropped: %s", s)
		}
	}
}

func TestMergeDedupe(t *testing.T) {
	startMenu := []Entry{
		{Name: "Discord", Kind: KindShortcut, Source: SourceStartMenu, Target: `C:\SM\Discord.lnk`, Resolved: `C:\Users\me\AppData\Local\Discord\Update.exe`, AUMID: "com.squirrel.Discord.Discord"},
		{Name: "Calculator", Kind: KindStore, Source: SourceStartMenu, Target: "Calc!App", AUMID: "Calc!App"},
		{Name: "Firefox", Kind: KindShortcut, Source: SourceStartMenu, Target: `C:\SM\Firefox.lnk`, Resolved: `C:\Program Files\Mozilla Firefox\firefox.exe`},
	}
	registry := []Entry{
		{Name: "Mozilla Firefox (x64 en-US)", Kind: KindExe, Source: SourceRegistry, Target: `c:\program files\mozilla firefox\FIREFOX.EXE`},
		{Name: "Discord", Kind: KindExe, Source: SourceRegistry, Target: `C:\Users\me\AppData\Local\Discord\app.ico.exe`},
		{Name: "GIMP", Kind: KindExe, Source: SourceRegistry, Target: `C:\Program Files\GIMP 2\bin\gimp.exe`},
	}
	apps := []Entry{
		{Name: "Calculator", Kind: KindStore, Source: SourceAppsFolder, Target: "calc!app", AUMID: "calc!app"},
		{Name: "Photos", Kind: KindStore, Source: SourceAppsFolder, Target: "Photos!App", AUMID: "Photos!App"},
	}
	everything := []Entry{
		{Name: "Discord", Kind: KindPortable, Source: SourceEverything, Target: `C:\Users\me\AppData\Local\Discord\app-1.0\Discord.exe`, Parent: "app-1.0"},
		{Name: "python", Kind: KindPortable, Source: SourceEverything, Target: `C:\Python312\python.exe`, Parent: "Python312"},
		{Name: "python", Kind: KindPortable, Source: SourceEverything, Target: `C:\Python311\python.exe`, Parent: "Python311"},
		{Name: "python", Kind: KindPortable, Source: SourceEverything, Target: `D:\copy\Python312\python.exe`, Parent: "Python312"},
		{Name: "python", Kind: KindPortable, Source: SourceEverything, Target: `D:\venv\Scripts\python.exe`, Parent: "Scripts"},
		{Name: "Note Taker", Kind: KindPortable, Source: SourceEverything, Target: `E:\Tools\Desk\NoteTaker.ahk`, Parent: "Desk"},
		{Name: "Note Taker", Kind: KindPortable, Source: SourceEverything, Target: `E:\Tools\Web\NoteTaker.ahk`, Parent: "Web"},
		{Name: "Pixel Paint Studio", Kind: KindPortable, Source: SourceEverything, Target: `E:\Tools\PixelPaintStudio\PixelPaintStudio.exe`, Parent: "PixelPaintStudio"},
	}

	got := Merge([][]Entry{startMenu, registry, apps, everything}, nil)
	want := []string{"Discord", "Calculator", "Firefox", "GIMP", "Photos", "python", "python", "python", "Note Taker", "Note Taker", "Pixel Paint Studio"}
	if g := names(got); len(g) != len(want) {
		t.Fatalf("got %v, want %v", g, want)
	} else {
		for i := range want {
			if g[i] != want[i] {
				t.Fatalf("got %v, want %v", g, want)
			}
		}
	}
	if got[0].Source != SourceStartMenu {
		t.Errorf("first source must win, got %s", got[0].Source)
	}
	for _, e := range got {
		if e.Name == "python" && !e.Generic {
			t.Errorf("python not generic: %+v", e)
		}
		if (e.Name == "Pixel Paint Studio" || e.Name == "Note Taker") && e.Generic {
			t.Errorf("%s: fewer than %d copies marked generic", e.Name, GenericMinCopies)
		}
	}
}
