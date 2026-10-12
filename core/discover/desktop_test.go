package discover

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const firefox = `# comment
[Desktop Entry]
Type=Application
Name=Firefox Web Browser
Name[de]=Firefox-Webbrowser
Exec=firefox %u
Icon=firefox
Path=/tmp\swork
NotShowIn=KDE;

[Desktop Action new-window]
Name=New Window
Exec=firefox --new-window %u
`

func TestParseDesktop(t *testing.T) {
	d := ParseDesktop(firefox, Locales("de_DE.UTF-8"))
	if d.Name != "Firefox-Webbrowser" || d.Exec != "firefox %u" || d.Path != "/tmp work" || d.Type != "Application" {
		t.Fatalf("%+v", d)
	}
	if d := ParseDesktop(firefox, nil); d.Name != "Firefox Web Browser" {
		t.Fatalf("unlocalized: %q", d.Name)
	}
	if !d.Visible([]string{"GNOME"}) || d.Visible([]string{"ubuntu", "kde"}) {
		t.Fatal("NotShowIn")
	}
	d.OnlyShowIn = []string{"XFCE"}
	if d.Visible([]string{"GNOME"}) {
		t.Fatal("OnlyShowIn")
	}
	for _, s := range []string{"NoDisplay=true", "Hidden=true", "Type=Link"} {
		if ParseDesktop("[Desktop Entry]\n"+s+"\nType=Application\nName=x\nExec=x\n", nil).Visible(nil) {
			t.Errorf("%s must hide", s)
		}
	}
}

func TestLocales(t *testing.T) {
	got := Locales("sr_RS.UTF-8@latin")
	want := []string{"sr_RS@latin", "sr_RS", "sr@latin", "sr"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if Locales("C.UTF-8") != nil || Locales("") != nil {
		t.Fatal("C locale has no translations")
	}
}

func TestDesktopIDAndPriority(t *testing.T) {
	if id := DesktopID("/usr/share/applications", "/usr/share/applications/kde/konsole.desktop"); id != "kde-konsole.desktop" {
		t.Fatal(id)
	}
	user, sys := t.TempDir(), t.TempDir()
	write := func(dir, name, body string) {
		p := filepath.Join(dir, "applications", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app := "[Desktop Entry]\nType=Application\nExec=true\nName="
	write(sys, "a.desktop", app+"System A")
	write(sys, "b.desktop", app+"B")
	write(sys, "c.desktop", app+"C\nTryExec=/nonexistent/ora-test")
	write(user, "a.desktop", app+"User A")
	write(user, "b.desktop", app+"B\nHidden=true")
	t.Setenv("XDG_DATA_HOME", user)
	t.Setenv("XDG_DATA_DIRS", sys)
	t.Setenv("LC_ALL", "C")
	es, err := Desktop{}.Discover(t.Logf)
	if err != nil || len(es) != 1 || es[0].Name != "User A" || es[0].Target != filepath.Join(user, "applications", "a.desktop") {
		t.Fatalf("%+v %v", es, err)
	}
}
