package gui

import "testing"

func TestRunCommand(t *testing.T) {
	got := runCommand(`C:\Windows\System32`, `C:\Program Files\ora\ora.exe`)
	want := `"C:\Windows\System32\conhost.exe" --headless "C:\Program Files\ora\ora.exe" --autostart`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
