package discover

import (
	"testing"

	"ora/core/config"
	"ora/core/index"
)

func TestClassifyShortcut(t *testing.T) {
	e := ClassifyShortcut(`C:\SM\Visual Studio Code.lnk`, `C:\Users\me\AppData\Local\Programs\Microsoft VS Code\Code.exe`, "", "", "Microsoft.VisualStudioCode")
	if e.Kind != index.KindShortcut || e.Name != "Visual Studio Code" || e.AUMID == "" {
		t.Errorf("win32 with AUMID: %+v", e)
	}
	e = ClassifyShortcut(`C:\SM\Discord.lnk`, `C:\d\Update.exe`, "--processStart Discord.exe", `C:\d`, "com.squirrel.Discord.Discord")
	if e.Kind != index.KindShortcut || e.Args == "" || e.Resolved == "" {
		t.Errorf("discord: %+v", e)
	}
	e = ClassifyShortcut(`C:\SM\Terminal.lnk`, "", "", "", "Microsoft.WindowsTerminal_8wekyb3d8bbwe!App")
	if e.Kind != index.KindStore || e.Target != "Microsoft.WindowsTerminal_8wekyb3d8bbwe!App" {
		t.Errorf("store without target: %+v", e)
	}
	e = ClassifyShortcut(`C:\SM\X.lnk`, `C:\Program Files\WindowsApps\X_1.0\x.exe`, "", "", "X_abc!App")
	if e.Kind != index.KindStore {
		t.Errorf("store under WindowsApps: %+v", e)
	}
	e = ClassifyShortcut(`C:\SM\Y.lnk`, `C:\Tools\y.exe`, "", "", "Y_abc!App")
	if e.Kind != index.KindShortcut {
		t.Errorf("packaged AUMID with real target must stay win32: %+v", e)
	}
}

func TestPickInstallExe(t *testing.T) {
	ex, _ := ExcludeRegexps(config.Everything{ExcludeRegex: config.DefaultExcludeRegex}, false)
	if got := PickInstallExe(`C:\App`, []string{"app.exe", "unins000.exe", "readme.txt"}, ex); got != `C:\App\app.exe` {
		t.Errorf("single exe: %q", got)
	}
	if got := PickInstallExe(`C:\App`, []string{"a.exe", "b.exe"}, ex); got != "" {
		t.Errorf("ambiguous must be empty: %q", got)
	}
	if got := PickInstallExe(`C:\App`, []string{"unins000.exe"}, ex); got != "" {
		t.Errorf("only uninstaller: %q", got)
	}
}

func TestPortableEntry(t *testing.T) {
	exts := []string{"exe", "ahk", "cmd", "bat"}
	e, ok := PortableEntry(`E:\Tools\PixelPaintStudio\PixelPaintStudio.exe`, exts)
	if !ok || e.Name != "Pixel Paint Studio" || e.Parent != "PixelPaintStudio" || e.Kind != index.KindPortable {
		t.Errorf("portable: %+v %v", e, ok)
	}
	for _, p := range []string{`E:\x\setup.exe`, `E:\x\unins000.exe`, `E:\x\notes.txt`, `C:\Users\me\Downloads\VSCodeUserSetup-x64-1.98.0.exe`, `E:\x\vc_redist.x64.exe`} {
		if _, ok := PortableEntry(p, exts); ok {
			t.Errorf("must drop %s", p)
		}
	}
}

func TestFileEntry(t *testing.T) {
	e := FileEntry(`D:\Docs\Configs\ProjectNotes.cfg`)
	if e.Kind != index.KindFile || e.Name != "Project Notes.cfg" || e.Parent != "Configs" {
		t.Errorf("file entry: %+v", e)
	}
	d := DirEntry(`D:\Docs\Configs`)
	if d.Kind != index.KindFolder || d.Name != "Configs" || d.Parent != "Docs" || d.Target != `D:\Docs\Configs` {
		t.Errorf("dir entry: %+v", d)
	}
}
