package config

import (
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestSetAliasKeepsCommentsAndKeys(t *testing.T) {
	in := "# my config\nmin_score: 0.9 # tuned\naliases:\n  code: Visual Studio Code\n"
	out, err := setAliasYAML([]byte(in), "ff", "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"# my config", "# tuned", "min_score: 0.9", "code: Visual Studio Code", "ff: Firefox"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestSetAliasOverwriteAndCreate(t *testing.T) {
	out, err := setAliasYAML(nil, "code", "Visual Studio Code")
	if err != nil {
		t.Fatal(err)
	}
	out, err = setAliasYAML(out, "code", "VSCodium")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]map[string]string
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if got := m["aliases"]["code"]; got != "VSCodium" || len(m["aliases"]) != 1 {
		t.Errorf("aliases = %v", m["aliases"])
	}

	out, err = setAliasYAML([]byte("aliases:\n"), "x", "Y")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "x: Y") {
		t.Errorf("null aliases not converted:\n%s", out)
	}
}

func TestNormalizeAliasKey(t *testing.T) {
	if k, err := NormalizeAliasKey("  Code "); err != nil || k != "code" {
		t.Errorf("got %q %v", k, err)
	}
	if _, err := NormalizeAliasKey("a.b"); err == nil {
		t.Error("dot must be rejected")
	}
	if _, err := NormalizeAliasKey(" "); err == nil {
		t.Error("empty must be rejected")
	}
}

func TestPushRecent(t *testing.T) {
	var ids []string
	for i := 0; i < 25; i++ {
		ids = PushRecent(ids, string(rune('a'+i)))
	}
	if len(ids) != recentMax || ids[0] != "y" {
		t.Errorf("len %d first %q", len(ids), ids[0])
	}
	ids = PushRecent(ids, "p")
	if ids[0] != "p" || len(ids) != recentMax {
		t.Errorf("move to front failed: %v", ids)
	}
}

func TestDefaultExcludeRegex(t *testing.T) {
	re := regexp.MustCompile("(?i)" + DefaultExcludeRegex)
	pf := regexp.MustCompile("(?i)" + ProgramFilesRegex)
	excluded := []string{
		`C:\Windows\System32\cmd.exe`,
		`C:\Users\me\AppData\Local\Temp\x\setup.exe`,
		`E:\proj\node_modules\.bin\tool.cmd`,
		`E:\proj\.git\hooks\pre-commit.bat`,
		`C:\ProgramData\Package Cache\{GUID}\vc_redist.exe`,
		`C:\$Recycle.Bin\S-1-5\$R123.exe`,
		`C:\Users\me\AppData\Local\Microsoft\WindowsApps\python.exe`,
		`C:\Users\me\AppData\Roaming\Code\User\History\-1a2b\x.ahk`,
		`C:\Users\me\AppData\Roaming\Kiro\User\globalStorage\ext\run.bat`,
		`C:\Users\me\go\pkg\mod\golang.org\x\tool.exe`,
		`C:\Users\me\AppData\Local\Programs\cursor\resources\app\codeBin\code.cmd`,
		`E:\proj\target\debug\build\x-1b92\build_script_build.exe`,
	}
	for _, p := range excluded {
		if !re.MatchString(p) {
			t.Errorf("not excluded: %s", p)
		}
	}
	if !pf.MatchString(`C:\Program Files\App\app.exe`) || !pf.MatchString(`d:\program files (x86)\App\app.exe`) {
		t.Error("program files not excluded")
	}
	kept := []string{
		`E:\Tools\PixelPaintStudio\PixelPaintStudio.exe`,
		`D:\Tools\Temperature\temp.exe`,
		`E:\Scripts\hotkeys.ahk`,
		`C:\Users\me\AppData\Local\Programs\Tool\tool.exe`,
		`E:\proj\target\release\mytool.exe`,
	}
	for _, p := range kept {
		if re.MatchString(p) || pf.MatchString(p) {
			t.Errorf("wrongly excluded: %s", p)
		}
	}
}
